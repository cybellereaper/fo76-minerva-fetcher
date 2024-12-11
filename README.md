# Fallout 76 Minerva Fetcher

A Go-based scraper that fetches data about Minerva's upcoming sales and current status from Fallout 76, and posts the information to a Discord webhook. This repository includes a GitHub Action to automate the scraping process and send updates regularly.

## Features

- Scrapes Minerva's current status and sale schedule from Fallout 76.
- Posts the data to a Discord webhook.
- Uses GitHub Actions to automate the process.

## Requirements

- Go 1.20+ installed on your local machine (if running manually).
- A Discord webhook URL for posting the data.
- GitHub repository secrets to store the Discord webhook URL.
