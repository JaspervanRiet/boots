package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/go-github/v52/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

func setupHttpClient() *http.Client {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")})
	client := oauth2.NewClient(context.Background(), token)
	return client
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error in loading .env file!")
	}

	httpClient := setupHttpClient()
	ghClient := github.NewClient(httpClient)

	repos, _, _ := ghClient.Repositories.List(context.Background(), "JaspervanRiet", nil)

	fmt.Println(repos)
}
