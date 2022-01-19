package k8s

import (
	"fmt"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"strings"
)

// ErrWatchNamespaceEnvVar indicates that the namespace environment variable is not set
var ErrWatchNamespaceEnvVar = fmt.Errorf("watch namespace env var must be set")

// GetNamespacesOptions returns the Options with Namespace the operator should be watching for changes
func GetNamespacesOptions() (*ctrl.Options, error) {
	options := ctrl.Options{
		Namespace: "",
	}
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"
	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return &options, ErrWatchNamespaceEnvVar
	}
	options.Namespace = ns

	// Multi Namespaces in WATCH_NAMESPACE (e.g ns1,ns2)
	if strings.Contains(ns, ",") {
		// configure cluster-scoped with MultiNamespacedCacheBuilder
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(ns, ","))
	}
	return &options, nil
}
