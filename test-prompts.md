# Assistant Test Prompts

## Filesystem
- List all files in the workspace
- Read task.md and summarize it
- Write a file called hello.txt with content "Hello World"
- Delete test.txt from the workspace

## Terminal
- Run `ls -la` and tell me what you see
- Execute `echo "hello from agent"` in the terminal
- Check what version of node and python are installed
- Run `date -u` and report the current UTC time

## Network
- Fetch https://example.com and save the response to a file
- Fetch https://httpbin.org/ip and tell me the IP
- Fetch https://jsonplaceholder.typicode.com/todos/1
- Fetch https://api.github.com/repos/anomalyco/opencode

## Mixed / Multi-step
- Fetch https://httpbin.org/uuid, save the UUID to a file called uuid.txt, then read it back
- List files, read the first one, and summarize its contents
- Run `node --version`, `python3 --version`, and `go version`, then write the results to versions.txt

## Reporting / Finalize
- List all files in the workspace and compose a bullet-point summary of everything present
- Read every .md file and give me a one-line summary of each
