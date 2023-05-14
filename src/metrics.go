package main

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v52/github"
	"github.com/montanaflynn/stats"
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

	var prStats []*pullRequestStatistics

	for _, pr := range prs {
		prStats = append(prStats, s.processPullRequest(ctx, pr, deployTimesForPullRequests))
	}

	untrackedPullRequests := 0
	reviewedPullRequests := 0
	deployedPullRequests := 0
	var reviewTime, timeToMerge, leadTimeForChanges []float64
	totalReviewTime := 0

	for _, stat := range prStats {
		if !stat.IsTrackedWithIssue {
			untrackedPullRequests++
		}

		if stat.WasReviewed {
			time := stat.TimeToReview.Hours()
			totalReviewTime += int(time)
			reviewTime = append(reviewTime, time)
			reviewedPullRequests++
		}

		timeToMerge = append(timeToMerge, float64(stat.TimeToMerge.Hours()))

		if stat.WasDeployed {
			deployedPullRequests++
			leadTimeForChanges = append(leadTimeForChanges, float64(stat.TimeToProduction.Hours()))
		}
	}

	numberOfPullRequests := len(prs)
	medianReviewTime, _ := stats.Median(reviewTime)
	medianTimeToMerge, _ := stats.Median(timeToMerge)
	medianLeadTimeForChanges, _ := stats.Median(leadTimeForChanges)

	return &Metrics{
		AverageReviewTime:        totalReviewTime / reviewedPullRequests,
		MedianReviewTime:         int(medianReviewTime),
		MedianTimeToMerge:        int(medianTimeToMerge),
		MedianLeadTimeForChanges: int(medianLeadTimeForChanges),
		TotalPullRequests:        numberOfPullRequests,
		PullRequestsWithoutIssue: untrackedPullRequests,
		PullRequestsWithReview:   reviewedPullRequests,
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

		allEvents = append(allEvents, timeline...)

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	// Sort by time desc
	sort.Slice(allEvents, func(i, j int) bool {
		timeA := allEvents[i].GetCreatedAt().Time
		if timeA.IsZero() {
			// Needed for reviews
			timeA = allEvents[i].GetSubmittedAt().Time
		}
		timeB := allEvents[j].GetCreatedAt().Time
		if timeB.IsZero() {
			timeB = allEvents[j].GetSubmittedAt().Time
		}

		// Note that there's still going to be some zero times,
		// but they are all commits, so we don't care.
		return timeA.Sub(timeB).Hours() > 0
	})

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
			// Do not save these, they will be deployed quickly
		} else {
			// Do not save times for undeployed pull requests
			if !timeForThisDeployment.IsZero() {
				timeDeployment[sha] = timeForThisDeployment
			}

		}

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
