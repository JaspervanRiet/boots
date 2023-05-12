package main

import (
	"context"
	"flag"
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

func getRepo() (*string, *string) {
	ownerName := flag.String("owner", "", "Specify the owner name")
	repoName := flag.String("repo", "", "Specify the repo name")
	flag.Parse()

	if *ownerName == "" || *repoName == "" {
		log.Fatal("Please specify an owner and repo!")
	}

	return ownerName, repoName
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error in loading .env file!")
	}

	owner, repo := getRepo()

	httpClient := setupHttpClient()
	ghClient := github.NewClient(httpClient)

	repos, _, _ := ghClient.PullRequests.List(
		context.Background(),
		*owner,
		*repo,
		&github.PullRequestListOptions{State: "closed"})

	for _, r := range repos {
		fmt.Println(*r.Title)
	}

}
