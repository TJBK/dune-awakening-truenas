package main

import (
	"bufio"
	"context"
	"os"
	"strings"
)

func readCommands(ctx context.Context) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			cmd := strings.ToLower(strings.TrimSpace(s.Text()))
			if cmd == "" {
				continue
			}
			select {
			case ch <- cmd:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}
