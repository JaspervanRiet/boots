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

type service struct {
	ghClient   *github.Client
	repository *Repository
}

type Repository struct {
	owner string
	name  string
}

func setupHttpClient() *http.Client {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")})
	client := oauth2.NewClient(context.Background(), token)
	return client
}

func getRepoToStudy() *Repository {
	ownerName := flag.String("owner", "", "Specify the owner name")
	repoName := flag.String("repo", "", "Specify the repo name")
	flag.Parse()

	if *ownerName == "" || *repoName == "" {
		log.Fatal("Please specify an owner and repo!")
	}

	return &Repository{owner: *ownerName, name: *repoName}
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
			if p.GetMergedAt().IsZero() {
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

	repo := getRepoToStudy()
	fmt.Println("Getting pull requests...")
	pullRequests := getPullRequestsFromLastTwoWeeks(ctx, ghClient, &repo.owner, &repo.name)

	service := MetricsService{
		ghClient:   ghClient,
		repository: repo,
	}
	fmt.Println("Analyzing pull requests...")
	metrics := service.AnalyzePullRequests(ctx, pullRequests)

	fmt.Println("-------")
	fmt.Println("METRICS")
	fmt.Println("-------")
	fmt.Println()
	fmt.Printf("Total pull requests:\t\t\t\t%d\n", metrics.TotalPullRequests)
	fmt.Printf("Untracked pull requests:\t\t\t%d\n", metrics.PullRequestsWithoutIssue)
	fmt.Printf("Pull requests with reviews:\t\t\t%d\n", metrics.PullRequestsWithReview)
	fmt.Printf("Review time (average):\t\t\t\t%d hours\n", metrics.AverageReviewTime)
	fmt.Printf("Review time (median):\t\t\t\t%d hours\n", metrics.MedianReviewTime)
	fmt.Printf("Time to merge (median):\t\t\t\t%d hours\n", metrics.MedianTimeToMerge)
	fmt.Printf("Lead time for changes (median):\t\t\t%d hours\n", metrics.MedianLeadTimeForChanges)

}

type Metrics struct {
	// Time for pull request open (review requested) till first review
	AverageReviewTime int
	// Time for pull request open (review requested) till first review
	MedianReviewTime int
	// Time for pull request open (review requested) till merge
	MedianTimeToMerge int
	// Time for pull request open (review requested) till in production
	MedianLeadTimeForChanges int
	TotalPullRequests        int
	PullRequestsWithoutIssue int
	PullRequestsWithReview   int
}
