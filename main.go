package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/joho/godotenv/autoload"
)

const (
	baseUrl = "https://openexchangerates.org/api/latest.json"
	refresh = 3 * time.Hour
)

type Rates struct {
	INR       float64   `json:"INR"`
	USD       float64   `json:"USD"`
	JPY       float64   `json:"JPY"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	rates      Rates
	ratesMutex sync.RWMutex
	apiKey     string
)

func main() {
	env := os.Getenv("ENV")
	if env == "" {
		env = "DEV"
	}
	hostUrl := os.Getenv("HOSTURL")

	apiKey = os.Getenv("OPENEXCHANGERATES_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENEXCHANGERATES_API_KEY env var not set :(")
	}

	loadRates()

	if rates.UpdatedAt.IsZero() || time.Since(rates.UpdatedAt) >= refresh {
		fetchRates()
	}

	go func() {
		ticker := time.NewTicker(refresh)
		for range ticker.C {
			fetchRates()
		}
	}()

	if strings.EqualFold(env, "RENDER") && hostUrl != "" {
		go func() {
			ticker := time.NewTicker(14 * time.Minute)
			for range ticker.C {
				func() {
					resp, err := http.Get(hostUrl + "/stay-alive")
					if err != nil {
						log.Println("Error in keep alive:", err)
					}
					defer resp.Body.Close()

					bodyBytes, err := io.ReadAll(resp.Body)
					if err != nil {
						log.Println("Error reading resp body in keep alive", err)
						return
					}
					log.Printf(string(bodyBytes))
				}()
			}
		}()
	}

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")

	r.GET("/", indexHandler)
	r.GET("/healthz", healthCheckHandler)
	r.GET("/stay-alive", stayAliveHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}

func fetchRates() {
	url := fmt.Sprintf("%s?app_id=%s&base=USD&symbols=INR,JPY", baseUrl, apiKey)
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error fetching rates:", err)
		return
	}

	defer resp.Body.Close()

	var result struct {
		Rates Rates `json:"rates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Println("Error decoding response:", err)
		return
	}

	ratesMutex.Lock()
	rates = result.Rates
	rates.USD = 1
	rates.UpdatedAt = time.Now()
	ratesMutex.Unlock()

	fmt.Println("Rates updated:", rates)
	storeRates()
}

func storeRates() {
	ratesMutex.RLock()
	defer ratesMutex.RUnlock()

	data, err := json.Marshal(rates)
	if err != nil {
		log.Println("Error marshalling rates:", err)
		return
	}

	err = os.WriteFile("rates.json", data, 0644)
	if err != nil {
		log.Println("Error writing rates to files:", err)
	}
}

func loadRates() {
	data, err := os.ReadFile("rates.json")
	if err != nil {
		log.Println("Error reading rates file:", err)
		return
	}

	ratesMutex.Lock()
	defer ratesMutex.Unlock()

	err = json.Unmarshal(data, &rates)
	if err != nil {
		log.Println("Error unmarshalling rates:", err)
	} else {
		log.Println("Loaded stored rates:", rates)
	}
}

func indexHandler(c *gin.Context) {
	ratesMutex.RLock()
	defer ratesMutex.RUnlock()
	c.HTML(http.StatusOK, "index.html", gin.H{
		"rates":      rates,
		"updated_at": rates.UpdatedAt.Format(time.RFC3339),
	})
}

func healthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func stayAliveHandler(c *gin.Context) {
	c.String(http.StatusOK, "ah ha ha ha stayin alive")
}
