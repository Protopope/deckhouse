/*
Copyright 2022 Flant JSC

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
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

var imagesDigestsTemplate = `// Code generated by "tools/images_tags.go" DO NOT EDIT.
// To generate run 'make generate'
package library

var	DefaultImagesDigests = map[string]interface{}{
{{- range $module, $images := . }}
	"{{ $module }}": map[string]interface{}{
  {{- range $image, $digest := $images }}
	  "{{ $image }}": "{{ $digest }}",
  {{- end }}
	},
{{- end }}
}
`

// This script is used for generating map of all modules and their images in deckhouse
// The dummy digests are assigned to all of them.
// This is used to execute templates and matrix tests.

func main() {
	var (
		output string
		stream = os.Stdout
	)
	flag.StringVar(&output, "output", "", "output file for generated code")
	flag.Parse()

	// If output defined write a file
	if output != "" {
		var err error
		stream, err = os.Create(output)
		if err != nil {
			panic(err)
		}

		defer stream.Close()
	}

	digests := make(map[string]map[string]string)

	// Run werf config render to get config file from which  we calculate images names
	cmd := exec.Command("werf", "config", "render", "--dev", "--log-quiet")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CI_COMMIT_REF_NAME=", "CI_COMMIT_TAG=", "WERF_ENV=FE", "SOURCE_REPO=", "GOPROXY=", "CRATESPROXY=", "NPMPROXY=", "PIPPROXY=")
	cmd.Dir = path.Join("..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		panic(err)
	}

	// Parse werf config and take images digests from it
	a := strings.NewReader(string(out))
	d := yaml.NewDecoder(a)
	for {
		spec := new(Spec)
		err := d.Decode(&spec)
		if spec == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			panic(err)
		}
		if spec.Image == "images-digests" {
			for _, dependency := range spec.Dependencies {
				for _, i := range dependency.Imports {
					s := strings.Split(i.TargetEnv, "_")
					module := s[3]
					digest := s[4]
					if digests[module] == nil {
						digests[module] = make(map[string]string)
					}
					// Set dummy tag imageHash-<moduleName>-<ImageName> for every image
					digests[module][digest] = fmt.Sprintf("imageHash-%s-%s", module, digest)
				}
			}
			break
		}
	}

	// Create template and execute it
	var buf bytes.Buffer
	t := template.New("imagesDigests")
	t, err = t.Parse(imagesDigestsTemplate)
	if err != nil {
		panic(err)
	}
	err = t.Execute(&buf, digests)
	if err != nil {
		panic(err)
	}
	// Run go fmt for generated code
	p, err := format.Source(buf.Bytes())
	if err != nil {
		panic(err)
	}
	stream.Write(p)
}

type Spec struct {
	Image        string       `yaml:"image"`
	Dependencies []Dependency `yaml:"dependencies"`
}

type Dependency struct {
	Imports []Import `yaml:"imports"`
}

type Import struct {
	TargetEnv string `yaml:"targetEnv"`
}
