/*
Copyright © 2020 Rex Via  l.rex.via@gmail.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/dhouti/sops-converter/pkg/decrypt"
	"go.uber.org/atomic"
	"os"

	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"strconv"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	secretsv1beta1 "github.com/dhouti/sops-converter/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SecretChecksumAnnotation = "secrets.dhouti.dev/secretChecksum"
	SopsChecksumAnnotation   = "secrets.dhouti.dev/sopsChecksum"
	OwnershipLabel           = "secrets.dhouti.dev/owned-by-controller"
	DeletionFinalizer        = "secrets.dhouti.dev/garbageCollection"
)

var lock sync.Mutex

// SopsSecretReconciler reconciles a SopsSecret object
type SopsSecretReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	decrypt.Decryptor
	finalizersDisabled *atomic.Bool
}

func (r *SopsSecretReconciler) InjectDecryptor(d decrypt.Decryptor) {
	r.Decryptor = d
}

// +kubebuilder:rbac:groups=secrets.dhouti.dev,resources=sopssecrets,verbs="*"
// +kubebuilder:rbac:groups=secrets.dhouti.dev,resources=sopssecrets/status,verbs="*"
// +kubebuilder:rbac:groups="",resources=secrets,verbs="*"

func (r *SopsSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("sopssecret", req.NamespacedName)
	r.initReconciler()

	// Attempt to fetch SopsSecret object. Short circuit if not exists
	obj := &secretsv1beta1.SopsSecret{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if k8serrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// If namespaces not set use namespace
	if len(obj.Spec.Template.Namespaces) == 0 {
		obj.Spec.Template.Namespaces = []string{
			obj.Namespace,
		}
	}

	r.checkFinalizersDisabled(obj)

	// Cleanup secrets in namespaces no longer in spec.
	ownershipLabelValue := fmt.Sprintf("%s.%s", obj.Name, obj.Namespace)
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.MatchingLabels{
		OwnershipLabel: ownershipLabelValue,
	}); err != nil {
		return ctrl.Result{}, err
	}

	for _, secretListItem := range secretList.Items {
		var foundItem bool
		for _, curNamespace := range obj.Spec.Template.Namespaces {
			if secretListItem.ObjectMeta.Namespace == curNamespace {
				foundItem = true
			}
		}
		if !foundItem {
			if err := r.Delete(ctx, &secretListItem); err != nil && !k8serrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
	}

	// Add finalizer if not set and not currently being deleted
	if obj.GetDeletionTimestamp().IsZero() && !controllerutil.ContainsFinalizer(obj, DeletionFinalizer) && !r.finalizersDisabled.Load() {
		controllerutil.AddFinalizer(obj, DeletionFinalizer)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update finalizers %v", err)
		}
		return ctrl.Result{}, nil // After add finalizer, requeue may not required ???
	}

	// Delete finalizer if finalizers are disabled
	if r.finalizersDisabled.Load() && controllerutil.ContainsFinalizer(obj, DeletionFinalizer) {
		controllerutil.RemoveFinalizer(obj, DeletionFinalizer)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to remove finalizers %v", err)
		}
		return ctrl.Result{}, nil // Owned objects are automatically garbage collected, Return and don't requeue ???
	}

	targetName := obj.Name
	if obj.Spec.Template.Name != "" {
		targetName = obj.Spec.Template.Name
	}

	var requeue bool
	for _, targetNamespace := range obj.Spec.Template.Namespaces {
		secretDestination := types.NamespacedName{
			Name:      targetName,
			Namespace: targetNamespace,
		}
		res, err := r.ReconcileNamespace(ctx, log, obj, secretDestination)
		// If there's an error return immediately
		if err != nil {
			return res, err
		}

		if res.Requeue {
			requeue = true
		}
	}

	return ctrl.Result{Requeue: requeue}, nil
}

func (r *SopsSecretReconciler) ReconcileNamespace(ctx context.Context, log logr.Logger, obj *secretsv1beta1.SopsSecret, secretDestination types.NamespacedName) (ctrl.Result, error) {
	// Fetch the secret
	// If ownership label not present on existing secret short circuit
	fetchSecret := &corev1.Secret{}
	err := r.Get(ctx, secretDestination, fetchSecret)
	secretNotFound := k8serrors.IsNotFound(err)
	if err != nil && !secretNotFound {
		return ctrl.Result{}, err
	}
	if !secretNotFound {
		_, ok := fetchSecret.Labels[OwnershipLabel]
		if !ok {
			// The secret does not have the ownership label, exit
			return ctrl.Result{}, nil
		}
	}

	dt := obj.GetDeletionTimestamp()
	// Object is being deleted
	if !dt.IsZero() {
		if controllerutil.ContainsFinalizer(obj, DeletionFinalizer) {
			// Delete the secret if it exists and finalizers enabled
			if !secretNotFound && !r.finalizersDisabled.Load() {
				err = r.Delete(ctx, fetchSecret)
				if err != nil {
					return ctrl.Result{}, err
				}
			}

			// Remove the finalizer and exit
			controllerutil.RemoveFinalizer(obj, DeletionFinalizer)
			if err = r.Update(ctx, obj); err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to remove finalizer, error: %v, while secretNotFound =  %t", err, secretNotFound)
			}
			log.Info("finalizer was removed...")
		}
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Calculate hashes of both objects to see if they are in desired state.
	secretDataBytes, err := json.Marshal(fetchSecret.Data)
	if err != nil {
		return ctrl.Result{}, err
	}

	currentSecretChecksum := hashItem(secretDataBytes)
	currentSopsChecksum := hashItem([]byte(obj.Data))

	// Handle annotations from template
	secretAnnotations := make(map[string]string)
	if obj.Spec.Template.Annotations != nil {
		secretAnnotations = obj.Spec.Template.Annotations
	}
	secretAnnotations[SecretChecksumAnnotation] = currentSecretChecksum
	secretAnnotations[SopsChecksumAnnotation] = currentSopsChecksum

	// Handle labels from template
	secretLabels := make(map[string]string)
	if obj.Spec.Template.Labels != nil {
		secretLabels = obj.Spec.Template.Labels
	}

	secretLabels[OwnershipLabel] = fmt.Sprintf("%s.%s", obj.Name, obj.Namespace)

	existingSecretChecksum, hasSecretChecksum := fetchSecret.Annotations[SecretChecksumAnnotation]
	existingSopsChecksum, hasSopsChecksum := fetchSecret.Annotations[SopsChecksumAnnotation]
	if hasSecretChecksum && hasSopsChecksum &&
		existingSecretChecksum == currentSecretChecksum &&
		existingSopsChecksum == currentSopsChecksum &&
		reflect.DeepEqual(fetchSecret.Annotations, secretAnnotations) &&
		reflect.DeepEqual(fetchSecret.Labels, secretLabels) {
		// That's one big if
		log.Info("Objects matched, skipping.")
		return ctrl.Result{}, nil
	}

	// Decrypt the Data field using Sops
	unencryptedData, err := r.Decrypt([]byte(obj.Data), "yaml")
	if err != nil {
		log.Error(err, "failed to decrypt data")
		return ctrl.Result{}, err
	}

	// Convert decryted secret into map[string]string, sadly cannot unmarshal directly into []byte
	secretDataStrings := make(map[string]string)
	err = yaml.Unmarshal(unencryptedData, &secretDataStrings)
	if err != nil {
		log.Error(err, "failed to unmarshal decrypted data")
		return ctrl.Result{}, err
	}

	// Convert map[string]string to map[string][]byte for compatibility with corev1.Secret
	generatedSecretData := make(map[string][]byte)
	for k, v := range secretDataStrings {
		generatedSecretData[k] = []byte(v)
	}

	// Add back ignored keys from live secret
	ignoredKeys := obj.Spec.IgnoredKeys
	if len(ignoredKeys) > 0 {
		for _, key := range ignoredKeys {
			existingKey, ok := fetchSecret.Data[key]
			if !ok {
				continue
			}

			generatedSecretData[key] = existingKey
		}
	}

	// Prevents an unnecessary reconcile on new objects
	secretDataBytes, err = json.Marshal(generatedSecretData)
	if err != nil {
		return ctrl.Result{}, err
	}
	currentSecretChecksum = hashItem(secretDataBytes)
	secretAnnotations[SecretChecksumAnnotation] = currentSecretChecksum

	generatedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretDestination.Name,
			Namespace: secretDestination.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, generatedSecret, func() error {
		generatedSecret.Annotations = secretAnnotations
		generatedSecret.Labels = secretLabels
		generatedSecret.Type = obj.Type

		generatedSecret.Data = generatedSecretData
		return nil
	})

	if err != nil {
		log.Error(err, "failed to apply changes to secret")
	}

	return ctrl.Result{}, err

}

func (r *SopsSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1beta1.SopsSecret{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldGeneration := e.ObjectOld.GetGeneration()
				newGeneration := e.ObjectNew.GetGeneration()
				// Generation is only updated on spec changes (also on deletion),
				// not metadata or status
				// Filter out events where the generation hasn't changed to
				// avoid being triggered by status updates
				return oldGeneration != newGeneration
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// The reconciler adds a finalizer so we perform clean-up
				// when the delete timestamp is added
				// Suppress Delete events to avoid filtering them out in the Reconcile function
				return false
			},
		}).
		// Use a WatchMap over an Ownerref, this should allow for safe deletion of the CRD and all objects without garbage collecting all of the secrets.
		// Would require scaling down the controller first.
		Watches(&source.Kind{Type: &corev1.Secret{}}, handler.EnqueueRequestsFromMapFunc(
			func(o client.Object) []reconcile.Request {
				ownershipLabel, ok := o.GetLabels()[OwnershipLabel]
				if !ok {
					return nil
				}

				splitOwnershipLabel := strings.Split(ownershipLabel, ".")
				if len(splitOwnershipLabel) != 2 {
					return nil
				}

				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      splitOwnershipLabel[0],
							Namespace: splitOwnershipLabel[1],
						},
					},
				}
			},
		)).
		Complete(r)
}

func hashItem(data []byte) string {
	hash := sha1.Sum(data)
	encodedHash := hex.EncodeToString(hash[:])
	return encodedHash
}

func (r *SopsSecretReconciler) checkFinalizersDisabled(obj *secretsv1beta1.SopsSecret) {
	lock.Lock()
	defer lock.Unlock()
	finalizersDisabledByEnv, _ := strconv.ParseBool(os.Getenv("DISABLE_FINALIZERS"))
	if finalizersDisabledByEnv || obj.Spec.SkipFinalizers {
		r.finalizersDisabled.Store(true)
	}
}

func (r *SopsSecretReconciler) initReconciler() {
	lock.Lock()
	defer lock.Unlock()
	// If not otherwise defined, default to the real decrypt func.
	if r.Decryptor == nil {
		r.Decryptor = &decrypt.SopsDecrytor{}
	}
	if r.finalizersDisabled == nil {
		r.finalizersDisabled = atomic.NewBool(false)
	}
}
