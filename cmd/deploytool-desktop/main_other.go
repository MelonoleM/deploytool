//go:build !windows

// Command deploytool-desktop is the native Windows desktop build. This stub keeps
// the package buildable on non-Windows platforms so `go test ./...` and CI work
// everywhere; the real entry point lives in main.go (//go:build windows).
package main

import "fmt"

func main() {
	fmt.Println("deploytool-desktop 仅在 Windows 上可用，请使用 `deploytool desktop` 在浏览器中打开控制台。")
}
