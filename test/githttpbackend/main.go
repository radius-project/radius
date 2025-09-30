package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultPort     = 3000
	defaultRepoRoot = "/var/lib/git"
)

type config struct {
	port     int
	repoRoot string
	username string
	password string
	seedRepo string
}

func main() {
	cfg := parseConfig()

	if err := os.MkdirAll(cfg.repoRoot, 0o775); err != nil {
		log.Fatalf("failed to create repository root %q: %v", cfg.repoRoot, err)
	}

	if err := enableReceivePack(); err != nil {
		log.Fatalf("failed to enable git receive-pack: %v", err)
	}

	if err := seedRepository(cfg.repoRoot, cfg.seedRepo); err != nil {
		log.Fatalf("failed to seed repository: %v", err)
	}

	backendPath, err := resolveGitHTTPBackend()
	if err != nil {
		log.Fatalf("unable to locate git-http-backend binary: %v", err)
	}

	log.Printf("Using git-http-backend at %s", backendPath)

	handler := &cgi.Handler{
		Path: backendPath,
		Dir:  cfg.repoRoot,
		Env: []string{
			fmt.Sprintf("GIT_PROJECT_ROOT=%s", cfg.repoRoot),
			"GIT_HTTP_EXPORT_ALL=true",
			"GIT_HTTP_MAX_REQUEST_BUFFER=1000M",
			"GIT_HTTP_BACKEND_ENABLE_RECEIVE_PACK=true",
			"GIT_HTTP_BACKEND_ENABLE_UPLOAD_PACK=true",
		},
	}

	var httpHandler http.Handler = handler
	if cfg.username != "" && cfg.password != "" {
		httpHandler = basicAuthMiddleware(httpHandler, cfg.username, cfg.password)
	} else {
		log.Printf("Basic authentication disabled (username or password not provided)")
	}

	httpHandler = loggingMiddleware(httpHandler)

	addr := fmt.Sprintf(":%d", cfg.port)
	log.Printf("Starting git HTTP server on %s with repository root %s", addr, cfg.repoRoot)
	if err := http.ListenAndServe(addr, httpHandler); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func parseConfig() config {
	var cfg config

	flag.IntVar(&cfg.port, "port", envInt("GIT_HTTP_PORT", defaultPort), "port to listen on")
	flag.StringVar(&cfg.repoRoot, "repo-root", envString("GIT_SERVER_TEMP_DIR", defaultRepoRoot), "directory containing git repositories")
	flag.StringVar(&cfg.username, "username", os.Getenv("GIT_USERNAME"), "basic auth username")
	flag.StringVar(&cfg.password, "password", os.Getenv("GIT_PASSWORD"), "basic auth password")
	flag.StringVar(&cfg.seedRepo, "seed-repo", os.Getenv("GIT_REPO_NAME"), "optional bare repository to initialize on startup")
	flag.Parse()

	if absPath, err := filepath.Abs(cfg.repoRoot); err == nil {
		cfg.repoRoot = absPath
	}

	return cfg
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("invalid value %q for %s, using default %d", raw, key, fallback)
		return fallback
	}

	return value
}

func enableReceivePack() error {
	cmd := exec.Command("git", "config", "--global", "http.receivepack", "true")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func seedRepository(root, repoName string) error {
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		return nil
	}

	repoPath := filepath.Join(root, fmt.Sprintf("%s.git", repoName))
	if info, err := os.Stat(repoPath); err == nil {
		if info.IsDir() {
			return os.Chmod(repoPath, 0o775)
		}
		return fmt.Errorf("seed path %s exists but is not a directory", repoPath)
	} else if !os.IsNotExist(err) {
		return err
	}

	cmd := exec.Command("git", "init", "--bare", repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git init --bare failed: %w", err)
	}

	return os.Chmod(repoPath, 0o775)
}

func resolveGitHTTPBackend() (string, error) {
	candidates := []string{}

	if execPath := strings.TrimSpace(os.Getenv("GIT_EXEC_PATH")); execPath != "" {
		candidates = append(candidates, filepath.Join(execPath, "git-http-backend"))
	}

	if len(candidates) == 0 {
		output, err := exec.Command("git", "--exec-path").Output()
		if err == nil {
			execPath := strings.TrimSpace(string(output))
			if execPath != "" {
				candidates = append(candidates, filepath.Join(execPath, "git-http-backend"))
			}
		}
	}

	candidates = append(candidates,
		"/usr/libexec/git-core/git-http-backend",
		"/usr/lib/git-core/git-http-backend",
		"/usr/local/libexec/git-core/git-http-backend",
	)

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("git-http-backend not found in expected locations: %v", candidates)
}

func basicAuthMiddleware(next http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suppliedUser, suppliedPassword, ok := r.BasicAuth()
		if !ok || suppliedUser != username || suppliedPassword != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
