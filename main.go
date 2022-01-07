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

package main

import (
	"flag"
	"fmt"
	secretsv1beta1 "github.com/dhouti/sops-converter/api/v1beta1"
	"github.com/dhouti/sops-converter/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"os"
	"os/exec"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strings"
	"time"
	// +kubebuilder:scaffold:imports
)

// watchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
// which specifies the namespaces (comma-separated) to watch.
// An empty value means the operator is running with cluster scope.
const watchNamespaceEnvVar = "WATCH_NAMESPACE"
const refreshGpgFmt = "echo %s | gpg --batch --always-trust --yes --passphrase-fd 0 --pinentry-mode=loopback -s $(mktemp)"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	done = make(chan bool)
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = secretsv1beta1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	klog.InitFlags(nil)
	flag.Parse()

	ctrl.SetLogger(klogr.New())
	initializeScheduleJob()

	mgr, err := initialConfiguration(metricsAddr)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	setupLog.Info("Gracefully shutdown...")
	done <- true //Gracefully shutdown
}

func initialConfiguration(metricsAddr string) (manager.Manager, error) {
	options, err := getOptions(metricsAddr)
	if err != nil {
		setupLog.Error(err, "unable to get WatchNamespace, "+
			"the manager will watch and manage resources in all Namespaces")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), *options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return nil, err
	}

	if err = (&controllers.SopsSecretReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("SopsSecret"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SopsSecret")
		return nil, err
	}

	return mgr, nil
}

func getOptions(metricsAddr string) (*ctrl.Options, error) {
	options := ctrl.Options{
		Namespace:          "",
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
	}

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return &options, fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	options.Namespace = ns

	// Multi Namespaces in WATCH_NAMESPACE (e.g ns1,ns2)
	if strings.Contains(ns, ",") {
		setupLog.Info("manager set up with multiple namespaces", "namespaces", ns)
		// configure cluster-scoped with MultiNamespacedCacheBuilder
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(ns, ","))
	}
	return &options, nil
}

func initializeScheduleJob() {
	if passPhrase, found := os.LookupEnv("PASSPHRASE"); found {
		ticker := time.NewTicker(60 * time.Minute)

		go func() {
			for {
				select {
				case <-done:
					ticker.Stop()
					return
				case <-ticker.C:
					out := cmd(fmt.Sprintf(refreshGpgFmt, passPhrase), true)
					klog.Info(string(out))
				}
			}
		}()
	}
}

func cmd(cmd string, shell bool) []byte {
	if shell {
		out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
		if err != nil {
			fmt.Println(err.Error())
		}
		return out
	} else {
		out, err := exec.Command(cmd).CombinedOutput()
		if err != nil {
			fmt.Println(err.Error())
		}
		return out
	}
}
