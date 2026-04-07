package parser

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"abitassistant/types"

	"github.com/PuerkitoBio/goquery"
)

// FetchApplicantData стягує дані про абітурієнта з abit-poisk
func FetchApplicantData(ctx context.Context, name string, tgID int64) ([]types.AbitPoiskApplicantEntry, error) {
	const apiURL = "https://abit-poisk.org.ua/api/statements"
	slog.Info("fetching abit-poisk data")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	data := url.Values{}
	data.Set("search", name)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")
	resp, err := client.Do(req)

	if err != nil {
		slog.Error("network error", "err", err)
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("bad response status", "status", resp.StatusCode)
		return nil, fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	// Abit-poisk повертає JSON з полем "html"
	var apiResponse struct {
		HTML string `json:"html"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		slog.Error("failed to decode json", "err", err)
		return nil, err
	}

	if apiResponse.HTML == "" {
		slog.Warn("empty HTML in response")
		return []types.AbitPoiskApplicantEntry{}, nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(apiResponse.HTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var results []types.AbitPoiskApplicantEntry
	doc.Find("table.table tbody tr").Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")

		if cells.Length() != 14 {
			return
		}

		entry := types.AbitPoiskApplicantEntry{
			Degree:                strings.TrimSpace(cells.Eq(0).Text()),
			FullName:              strings.TrimSpace(cells.Eq(1).Text()),
			Status:                strings.TrimSpace(cells.Eq(2).Text()),
			RankingNumber:         strings.TrimSpace(cells.Eq(3).Text()),
			Priority:              strings.TrimSpace(cells.Eq(4).Text()),
			TotalScore:            strings.TrimSpace(cells.Eq(6).Text()),
			EducationAvg:          strings.TrimSpace(cells.Eq(7).Text()),
			University:            strings.TrimSpace(cells.Eq(9).Text()),
			Faculty:               strings.TrimSpace(cells.Eq(10).Text()),
			Specialty:             strings.TrimSpace(cells.Eq(11).Text()),
			Quota:                 strings.TrimSpace(cells.Eq(12).Text()),
			OriginalDocsSubmitted: strings.TrimSpace(cells.Eq(13).Text()),
		}

		results = append(results, entry)

	})

	slog.Info("parsing complete", "count", len(results))
	return results, nil
}
