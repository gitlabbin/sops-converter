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

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes/scheme"

	secretsv1beta1 "github.com/dhouti/sops-converter/api/v1beta1"
)

type editOptions struct {
	args       []string
	TargetFile []byte
}

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Opens and decrypts a SopsSecret.",
	Long: `Opens a SopsSecret manifest, decrypts it,
		and loads it into the $EDITOR of your choice.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		o := &editOptions{}

		if err := o.validate(args); err != nil {
			return err
		}

		if err := o.complete(args); err != nil {
			return err
		}

		return o.process(args)
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}

func (o *editOptions) validate(args []string) error {
	if len(args) == 0 {
		log.Errorf("no filename given: %v", args)
		return fmt.Errorf("no filename given: %v", args)
	}
	if len(args) > 1 {
		log.Errorf("too many args: %v", args)
		return fmt.Errorf("too many args: %v", args)
	}

	return nil
}

func (o *editOptions) complete(args []string) error {
	o.args = args

	targetFile, err := ioutil.ReadFile(args[0])
	if err != nil {
		if os.IsNotExist(err) {
			// Create a new SopsSecret if not exists?
			return nil
		}
		return err
	}
	o.TargetFile = targetFile

	return nil
}

func (o *editOptions) process(args []string) error {
	var allDocuments []yaml.Node
	// Parse out multiple objects
	var originalYaml yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(o.TargetFile))
	for decoder.Decode(&originalYaml) == nil {
		allDocuments = append(allDocuments, originalYaml)
	}

	allObjects := map[int]*secretsv1beta1.SopsSecret{}
	for index, document := range allDocuments {
		// Convert back to yaml to parse again.
		documentBytes, err := yaml.Marshal(&document)
		if err != nil {
			log.Fatalf("[FATAL] yaml.Marshal error: %v", err)
			return err
		}

		// Convert manifest to runtime.Object to see if it's a SopsSecret
		m, _, err := scheme.Codecs.UniversalDeserializer().Decode(documentBytes, nil, nil)
		if err != nil {
			log.Warnf("decode failed on: %v", err)
			continue
		}

		// Assert that object is SopsSecret, if not exit
		sopsSecret, ok := m.(*secretsv1beta1.SopsSecret)
		if !ok {
			// Not a SopsSecret, skip
			continue
		}
		allObjects[index] = sopsSecret
	}

	if len(allObjects) == 0 {
		return errors.New("no SopsSecret objects found")
	}

	var targetIndex int
	if len(allObjects) > 1 {
		fmt.Printf("Found %v SopsSecret objects:\n", len(allObjects))
		fmt.Println("[index] name/namespace")
		for index, obj := range allObjects {
			fmt.Printf("[%v]: %s/%s\n", index, obj.Name, obj.Namespace)
		}
		fmt.Println("Enter the index of the SopsSecret you'd like to edit: ")
		fmt.Scanln(&targetIndex)
	}
	targetYamlMap := allDocuments[targetIndex]

	// Open a temporary file.
	tmpfile, err := ioutil.TempFile("", ".*.yml")
	if err != nil {
		return err
	}
	sopsSecret := allObjects[targetIndex]

	defer tmpfile.Close()
	defer os.Remove(tmpfile.Name())
	bytes.NewReader([]byte(sopsSecret.Data)).WriteTo(tmpfile)
	tmpfile.Sync()

	// Open sops editor directly
	sopsCommand := exec.Command("sops", tmpfile.Name())
	sopsCommand.Stdin = os.Stdin
	sopsCommand.Stdout = os.Stdout
	sopsCommand.Stderr = os.Stderr
	err = sopsCommand.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0

			// This works on both Unix and Windows. Although package
			// syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has
			// an ExitStatus() method with the same signature.
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				log.Printf("sops Exit Status: %d", status.ExitStatus())
			}
		} else {
			log.Errorf("sops cmd.Run failed on: %v", err)
		}
		return err
	}

	tmpfileContents, err := ioutil.ReadFile(tmpfile.Name())
	if err != nil {
		return err
	}

	// Fetch file mode so it's not changed on write.
	finfo, err := os.Stat(args[0])
	if err != nil {
		return err
	}

	// update data
	appIdx := -1
	for i, k := range targetYamlMap.Content[0].Content {
		if k.Value == "data" {
			appIdx = i + 1
			break
		}
	}
	targetYamlMap.Content[0].Content[appIdx].Value = string(tmpfileContents)

	allDocuments[targetIndex] = targetYamlMap
	var outBuffer bytes.Buffer
	for index, document := range allDocuments {
		if document.Content == nil {
			continue
		}
		out, err := yaml.Marshal(&document)
		if err != nil {
			log.Fatalf("yaml Marshal failed on: %v", err)
			return err
		}
		outBuffer.Write(out)
		if index < len(allDocuments)-1 {
			outBuffer.Write([]byte("---\n"))
		}
	}

	err = ioutil.WriteFile(args[0], outBuffer.Bytes(), finfo.Mode())
	if err != nil {
		return err
	}
	return nil
}
