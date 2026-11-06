//go:build ignore

// Test-only command recorder; never used by the developer tasks themselves.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	args := os.Args[1:]
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	trace, err := os.OpenFile(os.Getenv("TASK_TEST_TRACE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	err = json.NewEncoder(trace).Encode(map[string]any{
		"tool": name, "args": args, "cwd": cwd,
		"goos": os.Getenv("GOOS"), "goarch": os.Getenv("GOARCH"),
		"cgo": os.Getenv("CGO_ENABLED"), "assets": os.Getenv("KUBEBUILDER_ASSETS"),
	})
	if err != nil {
		panic(err)
	}
	if err := trace.Close(); err != nil {
		panic(err)
	}
	if name == "helm" {
		if os.Getenv("TASK_TEST_FAIL") == "helm" {
			os.Exit(44)
		}
		return
	}
	if os.Getenv("GOOS") != runtime.GOOS || os.Getenv("GOARCH") != runtime.GOARCH {
		panic("developer commands must target the local platform")
	}
	module := "."
	if len(args) > 1 && args[0] == "-C" {
		module, args = args[1], args[2:]
		if err := os.Chdir(module); err != nil {
			panic(err)
		}
	}
	switch {
	case len(args) > 0 && args[0] == "list":
		if args[len(args)-1] == "./pkg/version" {
			fmt.Println("version")
		} else {
			fmt.Println("main")
		}
	case len(args) > 0 && args[0] == "build":
		failure := os.Getenv("TASK_TEST_FAIL")
		if failure == "build" || (failure == "module" && module == "./test/testrp") {
			os.Exit(42)
		}
		fmt.Println("compiler stdout")
		fmt.Fprintln(os.Stderr, "compiler stderr")
		if len(args) == 3 && args[1] == "-v" && args[2] == "./pkg/version" {
			return
		}
		if len(args) < 4 || args[1] != "-o" {
			panic("expected a build with an output directory")
		}
		output, err := os.Stat(args[2])
		if err != nil || !output.IsDir() {
			panic(fmt.Sprintf("missing output directory: %v", err))
		}
		binary := filepath.Base(args[len(args)-1])
		if binary == "..." {
			binary = "rad"
			if module != "." {
				binary = filepath.Base(module)
			}
		}
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		if err := os.WriteFile(filepath.Join(args[2], binary), []byte("test binary"), 0o600); err != nil {
			panic(err)
		}
	case len(args) > 1 && args[1] == "setup-envtest":
		if os.Getenv("TASK_TEST_FAIL") == "setup-envtest" {
			os.Exit(45)
		}
		fmt.Println(os.Getenv("TASK_TEST_ASSETS"))
	case len(args) > 1 && args[1] == "gotestsum":
		if os.Getenv("TASK_TEST_FAIL") == "test" {
			os.Exit(43)
		}
	default:
		panic("unexpected command")
	}
}
