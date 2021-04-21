// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package confgenerator

import (
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/host"
)

const (
	validTestdataDir       = "testdata/valid"
	invalidTestdataDir     = "testdata/invalid"
	defaultLogsDir         = "/var/log/google-cloud-ops-agent/subagents"
	defaultStateDir        = "/var/lib/google-cloud-ops-agent/fluent-bit"
	windowsDefaultLogsDir  = "C:\\ProgramData\\Google\\Cloud Operations\\Ops Agent\\log"
	windowsDefaultStateDir = "C:\\ProgramData\\Google\\Cloud Operations\\Ops Agent\\run"
)

var (
	// Usage:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden
	// Add "-v" to show details for which files are updated with what:
	//   ops-agent$ go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden -v
	updateGolden       = flag.Bool("update_golden", false, "Whether to update the expected golden confs if they differ from the actual generated confs.")
	goldenMainPath     = validTestdataDir + "/%s/%s/golden_fluent_bit_main.conf"
	goldenParserPath   = validTestdataDir + "/%s/%s/golden_fluent_bit_parser.conf"
	goldenCollectdPath = validTestdataDir + "/%s/%s/golden_collectd.conf"
	goldenOtelPath     = validTestdataDir + "/%s/%s/golden_otel.conf"
	goldenErrorPath    = invalidTestdataDir + "/%s/%s/golden_error"
)

var platform string

func init() {
	hostInfo, _ := host.Info()
	if hostInfo.OS == "windows" {
		platform = "windows"
	} else {
		platform = "linux"
	}
}

func TestGenerateConfsWithValidInput(t *testing.T) {
	dirPath := validTestdataDir + "/" + platform
	logsDir := defaultLogsDir
	stateDir := defaultStateDir
	if platform == "windows" {
		logsDir = windowsDefaultLogsDir
		stateDir = windowsDefaultStateDir
	}
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			unifiedConfigFilePath := fmt.Sprintf(dirPath+"/%s/input.yaml", testName)
			// Special-case the default config.  It lives directly in the
			// confgenerator directory.  The golden files are still in the
			// testdata directory.
			if d.Name() == "default_config" {
				unifiedConfigFilePath = "default-config.yaml"
			} else if d.Name() == "windows_default_config" {
				unifiedConfigFilePath = "windows-default-config.yaml"
			}

			data, err := ioutil.ReadFile(unifiedConfigFilePath)
			if err != nil {
				t.Fatalf("ReadFile(%q) got %v", unifiedConfigFilePath, err)
			}
			uc, err := ParseUnifiedConfig(data)
			if err != nil {
				t.Fatalf("ParseUnifiedConfig got %v", err)
			}

			// Retrieve the expected golden conf files.
			expectedMainConfig := expectedConfig(testName, goldenMainPath, t)
			expectedParserConfig := expectedConfig(testName, goldenParserPath, t)
			// Generate the actual conf files.
			mainConf, parserConf, err := uc.GenerateFluentBitConfigs(logsDir, stateDir)
			if err != nil {
				t.Fatalf("GenerateFluentBitConfigs got %v", err)
			}
			// Compare the expected and actual and error out in case of diff.
			updateOrCompareGolden(t, testName, expectedMainConfig, mainConf, goldenMainPath)
			updateOrCompareGolden(t, testName, expectedParserConfig, parserConf, goldenParserPath)

			if platform == "windows" {
				expectedOtelConfig := expectedConfig(testName, goldenOtelPath, t)
				otelConf, err := uc.GenerateOtelConfig()
				if err != nil {
					t.Fatalf("GenerateOtelConfig got %v", err)
				}
				// Compare the expected and actual and error out in case of diff.
				updateOrCompareGolden(t, testName, expectedOtelConfig, otelConf, goldenOtelPath)
			} else {
				expectedCollectdConfig := expectedConfig(testName, goldenCollectdPath, t)
				collectdConf, err := uc.GenerateCollectdConfig(defaultLogsDir)
				if err != nil {
					t.Fatalf("GenerateCollectdConfig got %v", err)
				}
				// Compare the expected and actual and error out in case of diff.
				updateOrCompareGolden(t, testName, expectedCollectdConfig, collectdConf, goldenCollectdPath)
			}
		})
	}
}

func expectedConfig(testName string, validFilePathFormat string, t *testing.T) string {
	goldenPath := fmt.Sprintf(validFilePathFormat, platform, testName)
	rawExpectedConfig, err := ioutil.ReadFile(goldenPath)
	if err != nil {
		if *updateGolden {
			return ""
		} else {
			t.Fatalf("test %q: error reading the golden conf from %s : %s", testName, goldenPath, err)
		}
	}
	return string(rawExpectedConfig)
}

func expectedError(testName string, filePath string, t *testing.T) string {
	rawExpectedConfig, err := ioutil.ReadFile(filePath)
	if err != nil {
		if *updateGolden {
			return ""
		} else {
			t.Fatalf("test %q: error reading the expected error file from %s : %s", testName, filePath, err)
		}
	}
	return string(rawExpectedConfig)
}

func updateOrCompareGolden(t *testing.T, testName string, expected string, actual string, path string) {
	t.Helper()
	expected = strings.ReplaceAll(expected, "\r\n", "\n")
	actual = strings.ReplaceAll(actual, "\r\n", "\n")
	if diff := cmp.Diff(actual, expected); diff != "" {
		if *updateGolden {
			// Update the expected to match the actual.
			goldenPath := fmt.Sprintf(path, platform, testName)
			t.Logf("Detected -update_golden flag. Rewriting the %q golden file to apply the following diff\n%s.", goldenPath, diff)
			if err := ioutil.WriteFile(goldenPath, []byte(actual), 0644); err != nil {
				t.Fatalf("error updating golden file at %q : %s", goldenPath, err)
			}
		} else {
			t.Fatalf("conf mismatch (-got +want):\n%s", diff)
		}
	}
}

func TestGenerateConfigsWithInvalidInput(t *testing.T) {
	dirPath := invalidTestdataDir + "/" + platform
	dirs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		testName := d.Name()
		t.Run(testName, func(t *testing.T) {
			unifiedConfigFilePath := fmt.Sprintf(dirPath+"/%s/input.yaml", testName)
			expectedErrorFilePath := fmt.Sprintf(dirPath+"/%s/golden_error", testName)
			data, err := ioutil.ReadFile(unifiedConfigFilePath)
			expectedError := strings.TrimSuffix(expectedError(testName, expectedErrorFilePath, t), "\n")
			uc, err := ParseUnifiedConfig(data)
			if err != nil {
				updateOrCompareGolden(t, testName, expectedError, err.Error(), goldenErrorPath)
				// t.Errorf("test %q: generateConfigs failed with unexpected error.\nwant error\n  %s\ngot error:\n  %s\ninput yaml file:\n%s", testName, expected, err, data)
				// Unparsable config is a success for this test
				return
			}
			err = generateConfigs(uc, defaultLogsDir, defaultStateDir)
			if err == nil {
				t.Errorf("test %q: generateConfigs succeeded, want error. file:\n%s", testName, data)
			} else {
				updateOrCompareGolden(t, testName, expectedError, err.Error(), goldenErrorPath)
			}
		})
	}
}

func generateConfigs(uc UnifiedConfig, defaultLogsDir string, defaultStateDir string) (err error) {
	if _, _, err := uc.GenerateFluentBitConfigs(defaultLogsDir, defaultStateDir); err != nil {
		return err
	}
	if platform == "windows" {
		if _, err := uc.GenerateOtelConfig(); err != nil {
			return err
		}
	} else {
		if _, err := uc.GenerateCollectdConfig(defaultLogsDir); err != nil {
			return err
		}
	}
	return nil
}
