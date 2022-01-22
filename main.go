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
	"github.com/bombsimon/logrusr/v2"
	secretsv1beta1 "github.com/dhouti/sops-converter/api/v1beta1"
	"github.com/dhouti/sops-converter/controllers"
	"github.com/dhouti/sops-converter/pkg/exec"
	"github.com/dhouti/sops-converter/pkg/k8s"
	"github.com/dhouti/sops-converter/pkg/logger"
	"github.com/dhouti/sops-converter/pkg/version"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"os"
	goruntime "runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	"time"
	// +kubebuilder:scaffold:imports
)

// watchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
// which specifies the namespaces (comma-separated) to watch.
// An empty value means the operator is running with cluster scope.
const (
	cleanGpgTmp   = "find /tmp -name \"tmp.*\" -type f -mmin +30 -exec rm {} \\;" // 30 minutes
	refreshGpgFmt = "echo %s | gpg --batch --always-trust --yes --passphrase-fd 0 --pinentry-mode=loopback -s $(mktemp)"
)

var (
	scheme      = runtime.NewScheme()
	metricsAddr = ":8080"
	done        = make(chan bool)
	log         = logrusr.New(
		logger.GenerateLogger(),
		logrusr.WithReportCaller(),
	).WithCallDepth(0)
)

func init() {
	logger.ConfigControllerLog()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = secretsv1beta1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func printVersion() {
	log.Info(fmt.Sprintf("Version: %s", version.AppVersion))
	log.Info(fmt.Sprintf("Go Version: %s", goruntime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", goruntime.GOOS, goruntime.GOARCH))
	log.Info(fmt.Sprintf("Git Commit: %s", version.GitCommit))
	log.Info(fmt.Sprintf("BuildDate: %s", version.BuildDate))
}

func main() {
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.Parse()
	printVersion()

	initializeScheduleJob()

	mgr, err := initialConfiguration()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "problem running manager")
		os.Exit(1)
	}

	log.Info("Gracefully shutdown...")
	if _, found := os.LookupEnv("PASSPHRASE"); found {
		done <- true //Gracefully shutdown
	}
}

func initialConfiguration() (manager.Manager, error) {
	options, err := getOptions()
	if err != nil {
		log.Error(err, "unable to get WatchNamespace, "+
			"the manager will watch and manage resources in all Namespaces")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), *options)
	if err != nil {
		log.Error(err, "unable to start manager")
		return nil, err
	}

	if err = (&controllers.SopsSecretReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("SopsSecret"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create controller", "controller", "SopsSecret")
		return nil, err
	}

	return mgr, nil
}

func getOptions() (*ctrl.Options, error) {
	options, _ := k8s.GetNamespacesOptions()
	options.Scheme = scheme
	options.MetricsBindAddress = metricsAddr

	return options, nil
}

func initializeScheduleJob() {
	if passPhrase, found := os.LookupEnv("PASSPHRASE"); found {
		ticker := time.NewTicker(9 * time.Minute) //default-cache-ttl 600 seconds

		go func() {
			for {
				select {
				case <-done:
					ticker.Stop()
					log.Info("scheduler stopped...")
					return
				case <-ticker.C:
					out := exec.Cmd(cleanGpgTmp, true)
					log.Info("clean tmp done.", string(out))
					out = exec.Cmd(fmt.Sprintf(refreshGpgFmt, passPhrase), true)
					log.Info("refresh gpg session done.", string(out))
				}
			}
		}()
	}
}
