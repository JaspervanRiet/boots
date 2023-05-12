package main

import (
	"context"
	"fmt"

	"github.com/google/go-github/v52/github"
)

func main() {
	client := github.NewClient(nil)
	repos, _, _ := client.Repositories.List(context.Background(), "JaspervanRiet", nil)
	fmt.Println(repos)
}
