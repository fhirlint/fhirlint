package registry_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fhirlint/fhirlint/internal/registry"
)

// server answers every path with status, and with body when status is 200.
func server(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = io.WriteString(w, body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, registries []string) (string, string, error) {
	t.Helper()
	resp, base, err := registry.Get(context.Background(), http.DefaultClient, registries, "some.pkg", "application/json")
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body), base, nil
}

func TestGet_PrimaryAnswers(t *testing.T) {
	primary := server(t, http.StatusOK, "from primary")
	secondary := server(t, http.StatusOK, "from secondary")

	body, base, err := get(t, []string{primary.URL, secondary.URL})
	if err != nil {
		t.Fatal(err)
	}
	if body != "from primary" || base != primary.URL {
		t.Errorf("got %q from %s, want the primary", body, base)
	}
}

func TestGet_FallsBackOn404(t *testing.T) {
	primary := server(t, http.StatusNotFound, "")
	secondary := server(t, http.StatusOK, "from secondary")

	body, base, err := get(t, []string{primary.URL, secondary.URL})
	if err != nil {
		t.Fatal(err)
	}
	if body != "from secondary" || base != secondary.URL {
		t.Errorf("got %q from %s, want the secondary", body, base)
	}
}

func TestGet_NotFoundOnlyWhenEveryRegistrySays404(t *testing.T) {
	primary := server(t, http.StatusNotFound, "")
	secondary := server(t, http.StatusNotFound, "")

	_, _, err := get(t, []string{primary.URL, secondary.URL})
	if !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGet_A404NextToAFailureIsNotNotFound(t *testing.T) {
	// One registry says the package does not exist, the other could not say
	// anything. That is not evidence of absence — it must surface as the
	// failure, in both orders.
	for name, order := range map[string][2]int{
		"failure then 404": {http.StatusInternalServerError, http.StatusNotFound},
		"404 then failure": {http.StatusNotFound, http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			first := server(t, order[0], "")
			second := server(t, order[1], "")

			_, _, err := get(t, []string{first.URL, second.URL})
			if err == nil {
				t.Fatal("want an error")
			}
			if errors.Is(err, registry.ErrNotFound) {
				t.Errorf("err = %v, must not be ErrNotFound", err)
			}
		})
	}
}

func TestGet_TransportFailureFallsThrough(t *testing.T) {
	dead := server(t, http.StatusOK, "")
	deadURL := dead.URL
	dead.Close()
	secondary := server(t, http.StatusOK, "from secondary")

	body, base, err := get(t, []string{deadURL, secondary.URL})
	if err != nil {
		t.Fatal(err)
	}
	if body != "from secondary" || base != secondary.URL {
		t.Errorf("got %q from %s, want the secondary", body, base)
	}
}

func TestGet_ReportsTheLastFailureWhenNothingAnswers(t *testing.T) {
	first := server(t, http.StatusBadGateway, "")
	second := server(t, http.StatusInternalServerError, "")

	_, _, err := get(t, []string{first.URL, second.URL})
	if err == nil || errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
	if got, want := err.Error(), registry.Host(second.URL)+": HTTP 500"; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}

func TestDefaultOrderMatchesTheValidator(t *testing.T) {
	// PackageServer.defaultServers() in org.hl7.fhir.utilities: PRIMARY_SERVER
	// is packages2, SECONDARY_SERVER is packages.fhir.org. Same order here, or
	// audit and validate disagree about what exists.
	got := registry.Default()
	if len(got) != 2 || got[0] != "https://packages2.fhir.org/packages" || got[1] != "https://packages.fhir.org" {
		t.Errorf("Default() = %v", got)
	}
	got[0] = "changed"
	if registry.Default()[0] == "changed" {
		t.Error("Default() must return a fresh slice")
	}
}

func TestHosts(t *testing.T) {
	if got, want := registry.Hosts(nil), "packages2.fhir.org, packages.fhir.org"; got != want {
		t.Errorf("Hosts(nil) = %q, want %q", got, want)
	}
	if got := registry.Host("http://127.0.0.1:8080/x"); got != "127.0.0.1:8080" {
		t.Errorf("Host = %q", got)
	}
}
