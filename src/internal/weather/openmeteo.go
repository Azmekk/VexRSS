package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	forecastURL  = "https://api.open-meteo.com/v1/forecast"
	geocodingURL = "https://geocoding-api.open-meteo.com/v1/search"
	cacheTTL     = 10 * time.Minute
)

// Current is the shape returned to the frontend by /api/weather.
type Current struct {
	Temperature float64 `json:"temperature"`
	WeatherCode int     `json:"weatherCode"`
	WindSpeed   float64 `json:"windSpeed"`
	IsDay       int     `json:"isDay"`
	Units       string  `json:"units"`
	Label       string  `json:"label,omitempty"`
	FetchedAt   string  `json:"fetchedAt"`
}

type cacheEntry struct {
	data Current
	at   time.Time
}

// Client fetches current weather from Open-Meteo with a tiny in-memory cache.
type Client struct {
	HTTP    *http.Client
	mu      sync.Mutex
	entries map[string]cacheEntry
}

func New() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 8 * time.Second},
		entries: make(map[string]cacheEntry),
	}
}

// Fetch returns the current weather for lat/lon, using a cached value when
// fresh. lat and lon are rounded to 2 decimal places for the cache key.
func (c *Client) Fetch(ctx context.Context, lat, lon float64) (Current, error) {
	key := fmt.Sprintf("%.2f:%.2f", round2(lat), round2(lon))

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && time.Since(e.at) < cacheTTL {
		c.mu.Unlock()
		return e.data, nil
	}
	c.mu.Unlock()

	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', 4, 64))
	q.Set("current", "temperature_2m,weather_code,wind_speed_10m,is_day")
	q.Set("temperature_unit", "celsius")
	q.Set("wind_speed_unit", "kmh")
	q.Set("timezone", "auto")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forecastURL+"?"+q.Encode(), nil)
	if err != nil {
		return Current{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Current{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Current{}, fmt.Errorf("open-meteo %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw struct {
		Current struct {
			Temp2m      float64 `json:"temperature_2m"`
			WeatherCode int     `json:"weather_code"`
			WindSpeed   float64 `json:"wind_speed_10m"`
			IsDay       int     `json:"is_day"`
		} `json:"current"`
		CurrentUnits struct {
			Temp2m string `json:"temperature_2m"`
		} `json:"current_units"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Current{}, err
	}

	out := Current{
		Temperature: raw.Current.Temp2m,
		WeatherCode: raw.Current.WeatherCode,
		WindSpeed:   raw.Current.WindSpeed,
		IsDay:       raw.Current.IsDay,
		Units:       raw.CurrentUnits.Temp2m,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	c.mu.Lock()
	c.entries[key] = cacheEntry{data: out, at: time.Now()}
	c.mu.Unlock()

	return out, nil
}

// Geocode resolves a free-text place name to a lat/lon using Open-Meteo's
// geocoding service. Returns the first match or an error if none found.
func (c *Client) Geocode(ctx context.Context, place string) (lat, lon float64, label string, err error) {
	place = strings.TrimSpace(place)
	if place == "" {
		return 0, 0, "", errors.New("place is empty")
	}
	q := url.Values{}
	q.Set("name", place)
	q.Set("count", "1")
	q.Set("language", "en")
	q.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, geocodingURL+"?"+q.Encode(), nil)
	if err != nil {
		return 0, 0, "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("geocoding %d", resp.StatusCode)
	}
	var raw struct {
		Results []struct {
			Name      string  `json:"name"`
			Country   string  `json:"country"`
			Admin1    string  `json:"admin1"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return 0, 0, "", err
	}
	if len(raw.Results) == 0 {
		return 0, 0, "", fmt.Errorf("no results for %q", place)
	}
	r := raw.Results[0]
	parts := []string{r.Name}
	if r.Admin1 != "" && r.Admin1 != r.Name {
		parts = append(parts, r.Admin1)
	}
	if r.Country != "" {
		parts = append(parts, r.Country)
	}
	return r.Latitude, r.Longitude, strings.Join(parts, ", "), nil
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
