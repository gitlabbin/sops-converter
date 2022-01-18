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
	"fmt"
	"github.com/dhouti/sops-converter/cli/cmd"
	"github.com/dhouti/sops-converter/cli/logger"
	log "github.com/sirupsen/logrus"
	goruntime "runtime"
)

var (
	AppVersion, BuildDate, GitCommit string
)

func init() {
	logger.ConfigureLogging(nil)
}

func printVersion() {
	log.Info(fmt.Sprintf("Version: %s", AppVersion))
	log.Info(fmt.Sprintf("Go Version: %s", goruntime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", goruntime.GOOS, goruntime.GOARCH))
	log.Info(fmt.Sprintf("Git Commit: %s", GitCommit))
	log.Info(fmt.Sprintf("BuildDate: %s", BuildDate))
}

func main() {
	printVersion()
	cmd.Execute()
}
