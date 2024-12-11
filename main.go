package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"golang.org/x/net/proxy"
)

type MinervaData struct {
	CurrentStatus CurrentStatus `json:"current_status"`
	SaleSchedule  []SaleInfo    `json:"sale_schedule"`
}

type CurrentStatus struct {
	NextLocation string `json:"next_location"`
	ArrivalTime  string `json:"arrival_time"`
}

type SaleInfo struct {
	SaleNumber string `json:"sale_number"`
	Location   string `json:"location"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	IsNext     bool   `json:"is_next"`
}

const (
	maxRetries = 3
	retryDelay = 5 * time.Second
)

func main() {
	var data *MinervaData
	var err error

	// Retry logic for scraping
	for i := 0; i < maxRetries; i++ {
		data, err = scrapeMinervaData()
		if err == nil && data != nil && len(data.SaleSchedule) > 0 {
			break
		}

		log.Printf("Attempt %d failed: %v. Retrying in %v...", i+1, err, retryDelay)
		time.Sleep(retryDelay)
	}

	if err != nil || data == nil || len(data.SaleSchedule) == 0 {
		log.Printf("All retry attempts failed. Setting GitHub Action failure status")
		os.Exit(1) // This will cause the GitHub Action to fail and retry
	}

	// Retry logic for Discord posting
	for i := 0; i < maxRetries; i++ {
		err = postToDiscord(data)
		if err == nil {
			break
		}

		log.Printf("Discord post attempt %d failed: %v. Retrying in %v...", i+1, err, retryDelay)
		time.Sleep(retryDelay)
	}

	if err != nil {
		log.Printf("Failed to post to Discord after %d attempts: %v", maxRetries, err)
		os.Exit(1)
	}

	printJSON(data)
}

func scrapeMinervaData() (*MinervaData, error) {
	minervaData := &MinervaData{
		SaleSchedule: make([]SaleInfo, 0),
	}

	c := setupCollector()

	// Add timeout to prevent hanging
	c.SetRequestTimeout(30 * time.Second)

	setupHandlers(c, minervaData)

	if err := c.Visit("https://www.falloutbuilds.com/fo76/minerva/"); err != nil {
		return nil, fmt.Errorf("failed to visit URL: %w", err)
	}

	// Validate scraped data
	if minervaData.CurrentStatus.NextLocation == "" || len(minervaData.SaleSchedule) == 0 {
		return nil, fmt.Errorf("failed to scrape required data")
	}

	return minervaData, nil
}

func setupCollector() *colly.Collector {
	dialer, err := proxy.SOCKS5("tcp", "localhost:9050", nil, proxy.Direct)
	if err != nil {
		log.Fatal("Failed to create SOCKS5 dialer:", err)
	}

	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
	)

	c.WithTransport(transport)

	return c
}

func setupHandlers(c *colly.Collector, data *MinervaData) {
	c.OnHTML("div.p-3", extractCurrentStatus(data))
	c.OnHTML("figure.is-style-stripes table tbody tr", extractSaleInfo(data))
	c.OnError(handleError())
}

func extractCurrentStatus(data *MinervaData) func(*colly.HTMLElement) {
	return func(e *colly.HTMLElement) {
		data.CurrentStatus = CurrentStatus{
			NextLocation: strings.TrimSpace(e.ChildText("strong.text-lightgreen")),
			ArrivalTime:  e.ChildAttr("div[data-minervacountdown]", "data-minervacountdown"),
		}
	}
}

func extractSaleInfo(data *MinervaData) func(*colly.HTMLElement) {
	return func(e *colly.HTMLElement) {
		sale := e.ChildText("td:nth-child(1)")
		location := strings.TrimSpace(strings.Split(e.ChildText("td:nth-child(2)"), "Next")[0])
		startDate := e.ChildText("td:nth-child(3)")
		endDate := e.ChildText("td:nth-child(4)")

		if sale != "" && location != "" && startDate != "" && endDate != "" {
			saleInfo := SaleInfo{
				SaleNumber: strings.TrimSpace(sale),
				Location:   location,
				StartDate:  strings.TrimSpace(startDate),
				EndDate:    strings.TrimSpace(endDate),
				IsNext:     e.DOM.HasClass("bg-dark"),
			}
			data.SaleSchedule = append(data.SaleSchedule, saleInfo)
		}
	}
}

func handleError() func(*colly.Response, error) {
	return func(r *colly.Response, err error) {
		log.Printf("Request URL: %v failed with response: %v\nError: %v\n",
			r.Request.URL, r, err)
	}
}

func postToDiscord(data *MinervaData) error {
	// Create the Discord webhook message
	discordWebhookURL := os.Getenv("DISCORD_WEBHOOK_URL") // Replace with your actual webhook URL
	message := fmt.Sprintf("Minerva's Current Status:\nLocation: %s\nArrival Time: %s\n\nUpcoming Sale Schedule:\n",
		data.CurrentStatus.NextLocation, data.CurrentStatus.ArrivalTime)

	for _, sale := range data.SaleSchedule {
		message += fmt.Sprintf("Sale %s at %s: %s to %s\n", sale.SaleNumber, sale.Location, sale.StartDate, sale.EndDate)
	}

	// Send the message to Discord
	payload := map[string]interface{}{
		"content": message,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequest("POST", discordWebhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

func printJSON(data *MinervaData) {
	jsonData, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		log.Fatal("Failed to marshal JSON:", err)
	}
	fmt.Println(string(jsonData))
}
