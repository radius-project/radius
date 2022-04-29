// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/stretchr/testify/assert"
)

func TestCertValidationUnauthorized(t *testing.T) {
	tests := []struct {
		armid    string
		expected string
	}{
		{
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourceGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			"Unauthorized\n",
		},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		r := mux.NewRouter()
		r.Path("/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		//create certiticate
		tm := time.Now()
		cert := armAuthenticator.Certificate{
			Certificate: "MIIEqDCCApACCQDPVpCZk5mUcDANBgkqhkiG9w0BAQsFADAWMRQwEgYDVQQDDAtleGFtcGxlLmNvbTAeFw0yMjA0MjkyMjE4NDdaFw0zMjA0MjYyMjE4NDdaMBYxFDASBgNVBAMMC2V4YW1wbGUuY29tMIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA0EuVMw+iWVz6RDHCZwP29hIi+CwRNN1HT79OqeLJr2fPuVb+xMuyrSp67sNk9pRNgAaDrwIRBKwRMy3ZwmxaPd37D/CowthOIk6FdTmswvuP7hUHugKhppMU/lJpu/3wGO45UrnmdgfLGByZNvsbgsuYMAwP1sizLfN0bCxDVOG1Ndhz6uqLRF93zKucDzJR1XM1kNwUJz8Ld3RvYnT+2nxCsqHS6pVM/LnlRhqAD0VxxBTv+wMrqH+O4kjC6ye03n9KyTRCKhOtdAGCBv0fGR2M9c4C6MJzANoUI1f07sIUAAu0meMri3IIW5Jv8fkfnjisreTMKHsWptBkK8qtX3EmdQ196whvI41uXIF683atkKjYgW2U2HN7rDR4rY4+mW5YWsaAz5ffqc0fY49RTNP8Y9z4osd8Yw6Jl0qFcqXxD8A33gFD/mW+QFrYmQE4qoQHwUYSlkce48fixjrr5AGwI5V3eBhHT8G6It70fcVhKrfDuXcCIxDAtMjm3hgshig7jAmNJUkGVXZpqAj9Z0ZcKJVjAlGRGb6ZwZ75kZ8bjrO3tD3SebLffokV46MPAXlQLO3zS1+ZPxgRtWs7lrAox6D6+sjmwlrIEz0B7APXjRsgviPYD6AJxr792BUUa/ApP7upXcNetWCE5POmub8z3vKso8f4muU393FtslMCAwEAATANBgkqhkiG9w0BAQsFAAOCAgEAY8kOdpgzdL2fyMi9EY3j2bzxq+P5731zD/vtvyEgkO9lcZ/n28zojIGIL/DCAyuZM8+RjosB1EvVD8AV+Q1cNYY4/gcCR8uC3zbf1ymVimLZc5X3CHCKSmvIu40RpzVxP1fdDjg4Yi9ZZ9uzSqP0bgrpNrNw6Re+LdYqU+b0Bj0Z16kAH01tCH4kRCUT9Ei/YXlmjwg045JfYn8/EyxwrMHwiz7cHueEYkOnuRAp1k+ElgWMrWc7h00YG5n+3CjSqN/yxcq6aQQ5wr20JYINS9sH8GegDXcoD8KlJWxizSAO3u/CriKCR5G9PaseF1YLDen7cmsovu6mZ5WrSk6Zht5wNaf5HD0pRIfHYg7VKWs3mMoij3X7KDqGuipzOHAAFcxZt31FcWdLInsZrV1WWA3HtgfThtrHrZ/CvwepxDYlxRYz9oKk3C1kinJIDe8aZIfHvbcssUNXnTzsRgTSgbgAR79iHYaeVUn8Cj7fHYiwtuhSiGpKKjaiHilETDCFs7H7Qhnv13OrHUalXFzE9da1KetAQdgDa3evJhrqC/NV1Bje11dscrUTEaIYeFbjL8AuAlQz3/0SYB4Tq4+1mabxbvkw2ZoyrFh9TX7RSdKU+1373HBCDxK/wdj56UtK6D+2qjCCS3COiqT6vFNWtff/1gefVs3J/FWENNAD2C4=",
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "934367bf1c97033f877db0f15cb1b586957d313",
		}
		armCertMgr := armAuthenticator.NewArmCertManager("https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01")
		armAuthenticator.ArmCertStore.Store("934367bf1c97033f877db0f15cb1b586957d313", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := LowercaseURLPath(r)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		req.Header.Set(IngressCertThumbprintHeader, "934367bf1c97033f877db0f15cb1b586957d312")
		handler.ServeHTTP(w, req)
		parsed := w.Body.String()
		assert.Equal(t, tt.expected, parsed)
	}
}

func TestCertValidationAuthorized(t *testing.T) {
	tests := []struct {
		armid    string
		expected string
	}{
		{
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourceGroups/proxy-rg/providers/Microsoft.Kubernetes/connectedClusters/mvm2a",
			"/subscriptions/1f43aef5-7868-4c56-8a7f-cb6822a75c0e/resourcegroups/proxy-rg/providers/microsoft.kubernetes/connectedclusters/mvm2a",
		},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		r := mux.NewRouter()
		r.Path("/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		//create certiticate
		tm := time.Now()
		cert := armAuthenticator.Certificate{
			Certificate: "MIIEqDCCApACCQDPVpCZk5mUcDANBgkqhkiG9w0BAQsFADAWMRQwEgYDVQQDDAtleGFtcGxlLmNvbTAeFw0yMjA0MjkyMjE4NDdaFw0zMjA0MjYyMjE4NDdaMBYxFDASBgNVBAMMC2V4YW1wbGUuY29tMIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA0EuVMw+iWVz6RDHCZwP29hIi+CwRNN1HT79OqeLJr2fPuVb+xMuyrSp67sNk9pRNgAaDrwIRBKwRMy3ZwmxaPd37D/CowthOIk6FdTmswvuP7hUHugKhppMU/lJpu/3wGO45UrnmdgfLGByZNvsbgsuYMAwP1sizLfN0bCxDVOG1Ndhz6uqLRF93zKucDzJR1XM1kNwUJz8Ld3RvYnT+2nxCsqHS6pVM/LnlRhqAD0VxxBTv+wMrqH+O4kjC6ye03n9KyTRCKhOtdAGCBv0fGR2M9c4C6MJzANoUI1f07sIUAAu0meMri3IIW5Jv8fkfnjisreTMKHsWptBkK8qtX3EmdQ196whvI41uXIF683atkKjYgW2U2HN7rDR4rY4+mW5YWsaAz5ffqc0fY49RTNP8Y9z4osd8Yw6Jl0qFcqXxD8A33gFD/mW+QFrYmQE4qoQHwUYSlkce48fixjrr5AGwI5V3eBhHT8G6It70fcVhKrfDuXcCIxDAtMjm3hgshig7jAmNJUkGVXZpqAj9Z0ZcKJVjAlGRGb6ZwZ75kZ8bjrO3tD3SebLffokV46MPAXlQLO3zS1+ZPxgRtWs7lrAox6D6+sjmwlrIEz0B7APXjRsgviPYD6AJxr792BUUa/ApP7upXcNetWCE5POmub8z3vKso8f4muU393FtslMCAwEAATANBgkqhkiG9w0BAQsFAAOCAgEAY8kOdpgzdL2fyMi9EY3j2bzxq+P5731zD/vtvyEgkO9lcZ/n28zojIGIL/DCAyuZM8+RjosB1EvVD8AV+Q1cNYY4/gcCR8uC3zbf1ymVimLZc5X3CHCKSmvIu40RpzVxP1fdDjg4Yi9ZZ9uzSqP0bgrpNrNw6Re+LdYqU+b0Bj0Z16kAH01tCH4kRCUT9Ei/YXlmjwg045JfYn8/EyxwrMHwiz7cHueEYkOnuRAp1k+ElgWMrWc7h00YG5n+3CjSqN/yxcq6aQQ5wr20JYINS9sH8GegDXcoD8KlJWxizSAO3u/CriKCR5G9PaseF1YLDen7cmsovu6mZ5WrSk6Zht5wNaf5HD0pRIfHYg7VKWs3mMoij3X7KDqGuipzOHAAFcxZt31FcWdLInsZrV1WWA3HtgfThtrHrZ/CvwepxDYlxRYz9oKk3C1kinJIDe8aZIfHvbcssUNXnTzsRgTSgbgAR79iHYaeVUn8Cj7fHYiwtuhSiGpKKjaiHilETDCFs7H7Qhnv13OrHUalXFzE9da1KetAQdgDa3evJhrqC/NV1Bje11dscrUTEaIYeFbjL8AuAlQz3/0SYB4Tq4+1mabxbvkw2ZoyrFh9TX7RSdKU+1373HBCDxK/wdj56UtK6D+2qjCCS3COiqT6vFNWtff/1gefVs3J/FWENNAD2C4=",
			NotAfter:    tm.Add(time.Minute * 15),
			NotBefore:   tm,
			Thumbprint:  "934367bf1c97033f877db0f15cb1b586957d313",
		}
		armCertMgr := armAuthenticator.NewArmCertManager("https://admin.api-dogfood.resources.windows-int.net/metadata/authentication?api-version=2015-01-01")
		armAuthenticator.ArmCertStore.Store("934367bf1c97033f877db0f15cb1b586957d313", cert)
		r.Use(ClientCertValidator(armCertMgr))
		handler := LowercaseURLPath(r)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		req.Header.Set(IngressCertThumbprintHeader, "934367bf1c97033f877db0f15cb1b586957d313")
		handler.ServeHTTP(w, req)
		parsed := w.Body.String()
		assert.Equal(t, tt.expected, parsed)
	}
}
