package catalog

import (
	"encoding/json"
	"testing"
)

func TestArtifactHubPackageJSON_trustFields(t *testing.T) {
	raw := `{
		"name": "argo-cd",
		"stars": 67,
		"official": true,
		"cncf": true,
		"signed": true,
		"deprecated": false,
		"production_organizations_count": 2,
		"repository": {
			"name": "argo",
			"url": "https://argoproj.github.io/argo-helm",
			"verified_publisher": true
		}
	}`
	var pkg artifactHubPackage
	if err := json.Unmarshal([]byte(raw), &pkg); err != nil {
		t.Fatal(err)
	}
	if !pkg.Signed || !pkg.Official || !pkg.CNCF || !pkg.Repository.VerifiedPublisher {
		t.Fatal("trust fields not parsed")
	}
	if pkg.ProductionOrganizationsCount != 2 {
		t.Fatalf("production orgs = %d", pkg.ProductionOrganizationsCount)
	}
}
