package reporter

import (
	"testing"

	"github.com/anchore/k8s-inventory/internal/config"
)

func TestBuildUrl(t *testing.T) {
	anchoreDetails := config.AnchoreInfo{
		URL:      "https://ancho.re",
		User:     "admin",
		Password: "foobar",
	}

	expectedURL := "https://ancho.re/v1/enterprise/kubernetes-inventory"
	actualURL, err := buildURL(anchoreDetails, 1)
	if err != nil || expectedURL != actualURL {
		t.Errorf("Failed to build URL:\nexpected=%s\nactual=%s", expectedURL, actualURL)
	}

	expectedURL = "https://ancho.re/v2/kubernetes-inventory"
	actualURL, err = buildURL(anchoreDetails, 2)
	if err != nil || expectedURL != actualURL {
		t.Errorf("Failed to build URL:\nexpected=%s\nactual=%s", expectedURL, actualURL)
	}
}
