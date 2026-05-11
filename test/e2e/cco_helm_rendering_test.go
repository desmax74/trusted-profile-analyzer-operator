/*
Copyright 2026.

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

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	fieldCloudProvider    = "cloudProvider"
	fieldCloudCredentials = "cloudCredentials"
	kindCredentialsReq    = "CredentialsRequest"
	ccoSecretName         = "test-release-cloud-creds"
	ccoNamespace          = "openshift-cloud-credential-operator"
	testBucket            = "trustify-storage"
	testRegionUSEast1     = "us-east-1"
)

func testDatabaseValues() map[string]interface{} {
	return map[string]interface{}{
		"host":     "postgres.test.svc",
		fieldName:  "testdb",
		"username": "testuser",
		"password": "testpass",
	}
}

func awsValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: "aws",
		fieldCloudCredentials: map[string]interface{}{
			"aws": map[string]interface{}{
				"statementEntries": []interface{}{
					map[string]interface{}{
						"effect":   "Allow",
						"action":   []interface{}{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"},
						"resource": "*",
					},
				},
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: testRegionUSEast1,
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

func gcpValues() map[string]interface{} {
	return map[string]interface{}{
		fieldAppDomain:     testAppDomain,
		fieldCloudProvider: "gcp",
		fieldCloudCredentials: map[string]interface{}{
			"gcp": map[string]interface{}{
				"permissions": []interface{}{
					"storage.objects.get",
					"storage.objects.create",
					"storage.objects.delete",
					"storage.buckets.get",
				},
			},
		},
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: "us-central1",
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}
}

// parseYAMLDoc parses a YAML document via JSON round-trip so that numeric
// types are float64 (compatible with k8s unstructured helpers).
func parseYAMLDoc(doc string) (map[string]interface{}, error) {
	var intermediate interface{}
	if err := yaml.Unmarshal([]byte(doc), &intermediate); err != nil {
		return nil, err
	}
	jsonBytes, err := json.Marshal(intermediate)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func findDocByKind(t *testing.T, docs []string, kind string) map[string]interface{} {
	t.Helper()
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil {
			continue
		}
		if obj["kind"] == kind {
			return obj
		}
	}
	return nil
}

func nestedString(obj map[string]interface{}, keys ...string) string {
	current := obj
	for i, key := range keys {
		if i == len(keys)-1 {
			v, _ := current[key].(string)
			return v
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func TestHelmRenderAWSCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsValues())
	docs := splitYAMLDocs(rendered)

	cr := findDocByKind(t, docs, kindCredentialsReq)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for AWS")

	metadata, _ := cr[fieldMetadata].(map[string]interface{})
	require.NotNil(t, metadata, "metadata should exist")
	assert.Equal(t, ccoSecretName, metadata[fieldName], "CredentialsRequest name should match release")
	assert.Equal(t, ccoNamespace, metadata[fieldNamespace], "CredentialsRequest should be in CCO namespace")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	assert.Equal(t, ccoSecretName, nestedString(spec, "secretRef", fieldName), "secretRef.name should match")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "AWSProviderSpec", providerSpec["kind"], "providerSpec should be AWSProviderSpec")
	assert.Equal(t, "cloudcredential.openshift.io/v1",
		providerSpec[fieldAPIVersion], "providerSpec apiVersion should match")

	entries, ok := providerSpec["statementEntries"].([]interface{})
	assert.True(t, ok, "statementEntries should be a list")
	assert.NotEmpty(t, entries, "statementEntries should not be empty")

	assert.NotContains(t, rendered, "GCPProviderSpec", "AWS config should not contain GCP provider")
	assert.NotContains(t, rendered, "predefinedRoles", "AWS config should not contain predefinedRoles")
}

func TestHelmRenderGCPCredentialsRequest(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, gcpValues())
	docs := splitYAMLDocs(rendered)

	cr := findDocByKind(t, docs, kindCredentialsReq)
	require.NotNil(t, cr, "CredentialsRequest should be rendered for GCP")

	metadata, _ := cr[fieldMetadata].(map[string]interface{})
	require.NotNil(t, metadata, "metadata should exist")
	assert.Equal(t, ccoSecretName, metadata[fieldName], "CredentialsRequest name should match release")
	assert.Equal(t, ccoNamespace, metadata[fieldNamespace], "CredentialsRequest should be in CCO namespace")

	spec, _ := cr[fieldSpec].(map[string]interface{})
	require.NotNil(t, spec, "spec should exist")

	providerSpec, _ := spec["providerSpec"].(map[string]interface{})
	require.NotNil(t, providerSpec, "providerSpec should exist")
	assert.Equal(t, "GCPProviderSpec", providerSpec["kind"], "providerSpec should be GCPProviderSpec")

	roles, ok := providerSpec["predefinedRoles"].([]interface{})
	assert.True(t, ok, "predefinedRoles should be a list")
	assert.NotEmpty(t, roles, "predefinedRoles should not be empty")

	assert.NotContains(t, rendered, "AWSProviderSpec", "GCP config should not contain AWS provider")
	assert.NotContains(t, rendered, "statementEntries", "GCP config should not contain statementEntries")
}

func TestHelmRenderNoCredentialsRequestWithoutCloudProvider(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: "test-bucket",
			fieldRegion: testRegionUSEast1,
			"accessKey": "test-key",
			"secretKey": "test-secret",
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}

	rendered := renderHelmChart(t, chartPath, values)
	assert.NotEmpty(t, rendered, "chart should render without cloudProvider")

	docs := splitYAMLDocs(rendered)
	cr := findDocByKind(t, docs, kindCredentialsReq)
	assert.Nil(t, cr, "CredentialsRequest should NOT be rendered without cloudProvider")
}

func TestHelmRenderStorageEnvVarsWithCCO(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	rendered := renderHelmChart(t, chartPath, awsValues())

	assert.Contains(t, rendered, ccoSecretName,
		"rendered output should reference CCO secret")
	assert.Contains(t, rendered, "aws_access_key_id",
		"rendered output should reference aws_access_key_id key")
	assert.Contains(t, rendered, "aws_secret_access_key",
		"rendered output should reference aws_secret_access_key key")

	docs := splitYAMLDocs(rendered)
	for _, doc := range docs {
		obj, err := parseYAMLDoc(doc)
		if err != nil {
			continue
		}
		if obj["kind"] != "Deployment" {
			continue
		}
		metadata, _ := obj[fieldMetadata].(map[string]interface{})
		if metadata == nil || !strings.Contains(metadata[fieldName].(string), fieldServer) {
			continue
		}

		spec, _ := obj[fieldSpec].(map[string]interface{})
		template, _ := spec["template"].(map[string]interface{})
		templateSpec, _ := template[fieldSpec].(map[string]interface{})
		containers, _ := templateSpec["containers"].([]interface{})
		require.NotEmpty(t, containers, "server deployment should have containers")

		container, _ := containers[0].(map[string]interface{})
		envList, _ := container["env"].([]interface{})
		require.NotEmpty(t, envList, "container should have env vars")

		accessKeyFound := false
		secretKeyFound := false
		for _, e := range envList {
			env, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			envName, _ := env[fieldName].(string)
			if envName == "TRUSTD_S3_ACCESS_KEY" {
				refName := nestedString(env, "valueFrom", "secretKeyRef", fieldName)
				refKey := nestedString(env, "valueFrom", "secretKeyRef", "key")
				assert.Equal(t, ccoSecretName, refName, "ACCESS_KEY should reference CCO secret")
				assert.Equal(t, "aws_access_key_id", refKey, "ACCESS_KEY should use correct key")
				accessKeyFound = true
			}
			if envName == "TRUSTD_S3_SECRET_KEY" {
				refName := nestedString(env, "valueFrom", "secretKeyRef", fieldName)
				refKey := nestedString(env, "valueFrom", "secretKeyRef", "key")
				assert.Equal(t, ccoSecretName, refName, "SECRET_KEY should reference CCO secret")
				assert.Equal(t, "aws_secret_access_key", refKey, "SECRET_KEY should use correct key")
				secretKeyFound = true
			}
		}
		assert.True(t, accessKeyFound, "TRUSTD_S3_ACCESS_KEY env var should be present")
		assert.True(t, secretKeyFound, "TRUSTD_S3_SECRET_KEY env var should be present")
		return
	}
	t.Fatal("server deployment not found in rendered output")
}

func TestHelmRenderStorageEnvVarsWithoutCCO(t *testing.T) {
	if testing.Short() {
		t.Skip(skipE2ETest)
	}

	chartPath := getChartPath(t)
	values := map[string]interface{}{
		fieldAppDomain: testAppDomain,
		fieldStorage: map[string]interface{}{
			fieldType:   "s3",
			fieldBucket: testBucket,
			fieldRegion: testRegionUSEast1,
			"accessKey": "explicit-access-key",
			"secretKey": "explicit-secret-key",
		},
		fieldDatabase: testDatabaseValues(),
		fieldMetrics:  map[string]interface{}{fieldEnabled: false},
		fieldTracing:  map[string]interface{}{fieldEnabled: false},
	}

	rendered := renderHelmChart(t, chartPath, values)

	assert.NotContains(t, rendered, "cloud-creds",
		"rendered output should NOT reference CCO secret when cloudProvider is not set")
	assert.Contains(t, rendered, "explicit-access-key",
		"rendered output should contain explicit access key")
	assert.Contains(t, rendered, "explicit-secret-key",
		"rendered output should contain explicit secret key")
}
