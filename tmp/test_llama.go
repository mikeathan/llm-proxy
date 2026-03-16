package main

import (
"context"
"fmt"
"os"
"os/exec"
)

func main() {
	cmd := exec.CommandContext(context.Background(), "/home/mikeathan/dev/llama.cpp/build/bin/llama-server",
		"-m", "/home/mikeathan/dev/models/qwen2.5-3b-instruct-q4_k_m.gguf",
		"--port", "9000",
		"--ctx-size", "1024",
		"--gpu-layers", "99",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("Starting llama-server...")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
