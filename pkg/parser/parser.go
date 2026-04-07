package parser

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/OlexiyOdarchuk/abitassistant/types"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

// AbitPoiskFetchApplicantData стягує дані про абітурієнта з abit-poisk
func AbitPoiskFetchApplicantData(ctx context.Context, name string) ([]types.ApplicantEntry, error) {
	const apiURL = "https://abit-poisk.org.ua/api/statements"
	slog.Info("fetching abit-poisk data", "name", name)

	var results []types.ApplicantEntry
	var parseErr error

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"),
	)

	c.WithTransport(&http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	})

	c.OnResponse(func(r *colly.Response) {
		var apiResponse struct {
			HTML string `json:"html"`
		}

		if err := json.Unmarshal(r.Body, &apiResponse); err != nil {
			parseErr = fmt.Errorf("failed to decode json: %w", err)
			return
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(apiResponse.HTML))
		if err != nil {
			parseErr = fmt.Errorf("failed to parse HTML from JSON: %w", err)
			return
		}

		doc.Find("table.table tbody tr").Each(func(i int, s *goquery.Selection) {
			cells := s.Find("td")
			if cells.Length() < 14 {
				return
			}

			entry := types.ApplicantEntry{
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
	})

	c.OnError(func(r *colly.Response, err error) {
		slog.Error("colly request error", "status", r.StatusCode, "err", err)
		parseErr = err
	})

	payload := map[string]string{"search": name}
	err := c.Post(apiURL, payload)

	if err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, parseErr
	}

	slog.Info("parsing complete", "count", len(results))
	return results, nil
}
