package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/go-github/v52/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

func setupHttpClient() *http.Client {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")})
	client := oauth2.NewClient(context.Background(), token)
	return client
}

func getRepoToStudy() (*string, *string) {
	ownerName := flag.String("owner", "", "Specify the owner name")
	repoName := flag.String("repo", "", "Specify the repo name")
	flag.Parse()

	if *ownerName == "" || *repoName == "" {
		log.Fatal("Please specify an owner and repo!")
	}

	return ownerName, repoName
}

func getPullRequestsFromLastTwoWeeks(ctx context.Context, ghClient *github.Client, owner *string, repo *string) []*github.PullRequest {
	var allPullRequests []*github.PullRequest

	opt := &github.PullRequestListOptions{
		State:       "closed",
		ListOptions: github.ListOptions{PerPage: 10},
	}

	now := time.Now()

	for {
		pullRequests, resp, err := ghClient.PullRequests.List(
			ctx,
			*owner,
			*repo,
			opt)

		if err != nil {
			log.Fatal("Encounted error!", err)
		}

		for _, p := range pullRequests {
			if p.MergedAt == nil {
				continue
			}
			weeksAgo := now.Sub(p.MergedAt.Time).Hours() / (24 * 7)
			if weeksAgo >= 2 {
				return allPullRequests
			}

			allPullRequests = append(allPullRequests, p)
		}

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	return allPullRequests
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error in loading .env file!")
	}

	ctx := context.Background()
	httpClient := setupHttpClient()
	ghClient := github.NewClient(httpClient)

	owner, repo := getRepoToStudy()
	pullRequests := getPullRequestsFromLastTwoWeeks(ctx, ghClient, owner, repo)

	totalTime := time.Duration(0)
	totalNumberOfMergedPullRequests := 0

	for _, pr := range pullRequests {
		commits, _, _ := ghClient.PullRequests.ListCommits(ctx, *owner, *repo, *pr.Number, &github.ListOptions{})
		firstCommitTime := commits[0].Commit.Author.Date
		mergeTime := *pr.MergedAt
		diff := mergeTime.Sub(firstCommitTime.Time)
		totalTime = totalTime + diff
		totalNumberOfMergedPullRequests = totalNumberOfMergedPullRequests + 1
	}

	metrics := Metrics{
		LeadTimeToMerge: (totalTime / time.Duration(totalNumberOfMergedPullRequests)).Round(time.Hour),
	}

	fmt.Println(fmt.Sprintf("LeadTimeToMerge %s", metrics.LeadTimeToMerge))
}

type Metrics struct {
	LeadTimeToMerge time.Duration
}
