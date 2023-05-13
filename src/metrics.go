package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/google/go-github/v52/github"
)

const branchPrefixNoIssue = "noticket"

const eventReviewRequested = "review_requested"
const eventReviewed = "reviewed"

const pullRequestStateClosed = "closed"

type MetricsService service

// Performs analysis on the given pull requests and returns the calculated metrics
func (s *MetricsService) AnalyzePullRequests(ctx context.Context, prs []*github.PullRequest) *Metrics {
	deployments, _, _ := s.ghClient.Repositories.ListDeployments(ctx, s.repository.owner, s.repository.name, &github.DeploymentsListOptions{})
	deployTimesForPullRequests := s.getDeploymentTimesForSHA(deployments, prs)

	var stats []*pullRequestStatistics

	for _, pr := range prs {
		stats = append(stats, s.processPullRequest(ctx, pr, deployTimesForPullRequests))
	}

	untrackedPullRequests := 0
	pullRequestsWithReview := 0
	reviewTime, timeToMerge, leadTimeForChanges := 0, 0, 0

	for _, stat := range stats {
		if !stat.IsTrackedWithIssue {
			untrackedPullRequests++
		}

		if stat.WasReviewed {
			reviewTime += int(stat.TimeToReview.Hours())
			pullRequestsWithReview++
		}

		timeToMerge += int(stat.TimeToMerge.Hours())

		if stat.WasDeployed {
			leadTimeForChanges += int(stat.TimeToProduction.Hours())
		}
	}

	numberOfPullRequests := len(prs)

	return &Metrics{
		ReviewTime:               reviewTime / numberOfPullRequests,
		TimeToMerge:              timeToMerge / numberOfPullRequests,
		LeadTimeForChanges:       leadTimeForChanges / numberOfPullRequests,
		TotalPullRequests:        numberOfPullRequests,
		PullRequestsWithoutIssue: untrackedPullRequests,
		PullRequestsWithReview:   pullRequestsWithReview,
	}

}

func (s *MetricsService) processPullRequest(ctx context.Context, pr *github.PullRequest, deployTimesForPullRequests map[string]time.Time) *pullRequestStatistics {
	timelineEvents := s.getAllTimelineEventsForPullRequest(ctx, pr)

	// Default to creation date in case no review was requested
	timeReviewRequested := pr.GetCreatedAt().Time
	isFirstReview := true
	wasReviewed := false

	var timeReviewed time.Time

	for _, e := range timelineEvents {
		if isFirstReview && e.GetEvent() == eventReviewRequested {
			timeReviewRequested = e.GetCreatedAt().Time
			isFirstReview = false
		}

		if timeReviewed.IsZero() && e.GetEvent() == eventReviewed {
			timeReviewed = e.GetSubmittedAt().Time
			wasReviewed = true

			// We have identified the first review, we know enough
			break
		}
	}

	timeMerged := pr.GetMergedAt()

	var timeDeployed time.Time
	if value, ok := deployTimesForPullRequests[pr.GetMergeCommitSHA()]; ok {
		timeDeployed = value
	}

	return &pullRequestStatistics{
		IsTrackedWithIssue:    s.doesPullRequestHaveIssueAttached(pr),
		TimeToReview:          timeReviewed.Sub(timeReviewRequested).Round(time.Hour),
		TimeToMerge:           timeMerged.Sub(timeReviewRequested).Round(time.Hour),
		TimeToProduction:      timeDeployed.Sub(timeReviewRequested).Round(time.Hour),
		WasClosedWithoutMerge: pr.GetState() == pullRequestStateClosed && !pr.GetMerged(),
		WasReviewed:           wasReviewed,
		WasDeployed:           !timeDeployed.IsZero(),
	}
}

// Returns all the timeline events for a pull request, e.g. review_requested.
func (s *MetricsService) getAllTimelineEventsForPullRequest(ctx context.Context, pr *github.PullRequest) []*github.Timeline {
	var allEvents []*github.Timeline

	opt := &github.ListOptions{
		PerPage: 10,
	}

	for {
		timeline, resp, err := s.ghClient.Issues.ListIssueTimeline(ctx, s.repository.owner, s.repository.name, *pr.Number, opt)

		if err != nil {
			log.Fatal("Encounted error!", err)
		}

		for _, t := range timeline {
			allEvents = append(allEvents, t)
		}

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	return allEvents
}

// Returns a map with as key the SHA of each pull request, and as value the time when that pull
// request was deployed
//
// Pull requests that were not deployed are not included in the map.
func (s *MetricsService) getDeploymentTimesForSHA(deployments []*github.Deployment, pullRequests []*github.PullRequest) map[string]time.Time {
	timeDeployment := make(map[string]time.Time)
	deploymentShas := make(map[string]time.Time)

	for _, d := range deployments {
		sha := d.GetSHA()
		deploymentShas[sha] = d.GetCreatedAt().Time
	}

	var timeForThisDeployment time.Time

	for _, pr := range pullRequests {
		sha := pr.GetMergeCommitSHA()
		if value, ok := deploymentShas[sha]; ok {
			timeForThisDeployment = value
		}
		timeDeployment[sha] = timeForThisDeployment
	}

	return timeDeployment
}

// Returns true if this pull request has a branch name that indicates being linked
// to an issue
func (s *MetricsService) doesPullRequestHaveIssueAttached(pr *github.PullRequest) bool {
	branch := *pr.Head.Label
	isTrackedWithIssue := !strings.Contains(branch, branchPrefixNoIssue)
	return isTrackedWithIssue
}

type pullRequestStatistics struct {
	// Time from ready for review (defined as first review request) till first actual review
	TimeToReview time.Duration

	// Time from review first requested till merged
	TimeToMerge time.Duration

	// Time from review first requested and PR appearing in production
	TimeToProduction      time.Duration
	IsTrackedWithIssue    bool
	WasClosedWithoutMerge bool
	WasReviewed           bool
	WasDeployed           bool
}
