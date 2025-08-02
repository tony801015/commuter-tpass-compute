package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type MetroData struct {
	StartSID         string `json:"StartSID"`
	EndSID           string `json:"EndSID"`
	StartStationName string `json:"StartStationName"`
	EndStationName   string `json:"EndStationName"`
	DeductedFare     string `json:"DeductedFare"`
	Discount60       string `json:"Discount60"`
	Discount40       string `json:"Discount40"`
	Lang             string `json:"Lang"`
}

type FareRequest struct {
	StartStationName string `json:"startStationName"`
	EndStationName   string `json:"endStationName"`
	IsRoundTrip      bool   `json:"isRoundTrip"`
	Trips            int    `json:"Trips"`
}
type FareResponse struct {
	TotalFare int `json:"totalFare"`
}

type Station struct {
	StationSID  string `json:"StationSID"`
	StationName string `json:"StationName"`
}

var stations []Station

func main() {
	// 加载mrt.json到stations变量
	err := loadStations()
	if err != nil {
		fmt.Println("Error loading mrt.json:", err)
		return
	}

	r := gin.Default()
	r.Use(CORSMiddleware())
	r.GET("/metrodata", func(c *gin.Context) {
		startName := c.DefaultQuery("startName", "")
		endName := c.DefaultQuery("endName", "")

		startsid, err := getSIDByName(startName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
		endsid, err := getSIDByName(endName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		data, err := fetchMetroData(startsid, endsid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, data)
	})

	// 新增的API，用于模糊查询站名
	r.GET("/searchstations", func(c *gin.Context) {
		query := c.DefaultQuery("query", "")
		matchingStations := searchStations(query)
		c.JSON(http.StatusOK, matchingStations)
	})

	r.POST("/calculatefare", func(c *gin.Context) {
		log.Printf("Received calculatefare request")

		var requests []FareRequest
		if err := c.BindJSON(&requests); err != nil {
			log.Printf("Error binding JSON: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request data",
			})
			return
		}

		log.Printf("Processing %d fare requests", len(requests))
		totalFare, err := calculateTotalFare(requests)
		if err != nil {
			log.Printf("Error calculating total fare: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		log.Printf("Successfully calculated total fare: %d", totalFare)
		c.JSON(http.StatusOK, FareResponse{
			TotalFare: totalFare,
		})
	})

	r.Run(":8080")
}

func loadStations() error {
	content, err := ioutil.ReadFile("mrt.json")
	if err != nil {
		return err
	}

	return json.Unmarshal(content, &stations)
}

func getSIDByName(name string) (string, error) {
	for _, station := range stations {
		if station.StationName == name {
			return station.StationSID, nil
		}
	}
	return "", fmt.Errorf("station not found: %s", name)
}

func fetchMetroData(startSID, endSID string) (*MetroData, error) {
	log.Printf("Fetching metro data for %s -> %s", startSID, endSID)

	// 先尝试从缓存中读取数据
	data, err := readFromCache(startSID, endSID)
	if err != nil {
		log.Printf("Error reading from cache: %v", err)
		return nil, err
	}

	if data != nil {
		log.Printf("Found cached data for %s -> %s", startSID, endSID)
		return data, nil // 如果缓存中有数据，直接返回缓存数据
	}

	log.Printf("No cached data found, fetching from API")

	// 如果缓存中没有数据，从第三方API获取
	url := "https://web.metro.taipei/apis/metrostationapi/ticketinfo"
	method := "POST"

	payload := strings.NewReader(fmt.Sprintf(`{
	    "StartSID": "%s",
	    "EndSID": "%s",
	    "Lang": "tw"
	}`, startSID, endSID))

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		log.Printf("Error creating HTTP request: %v", err)
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	log.Printf("Making API request to: %s", url)
	res, err := client.Do(req)
	if err != nil {
		log.Printf("Error making HTTP request: %v", err)
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return nil, err
	}

	log.Printf("API response status: %d", res.StatusCode)

	// 檢查 HTTP 狀態碼
	if res.StatusCode != http.StatusOK {
		log.Printf("API request failed with status %d: %s", res.StatusCode, string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", res.StatusCode, string(body))
	}

	// 檢查響應內容是否為 JSON
	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		// 如果不是 JSON，記錄響應內容的前 200 個字符用於調試
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		return nil, fmt.Errorf("API returned non-JSON response (Content-Type: %s). Response preview: %s", contentType, bodyPreview)
	}

	var metroData MetroData
	err = json.Unmarshal(body, &metroData)
	if err != nil {
		// 如果 JSON 解析失敗，記錄響應內容的前 200 個字符
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		log.Printf("Failed to parse JSON response: %v. Response preview: %s", err, bodyPreview)
		return nil, fmt.Errorf("failed to parse JSON response: %v. Response preview: %s", err, bodyPreview)
	}

	log.Printf("Successfully parsed metro data: StartSID=%s, EndSID=%s, Fare=%s",
		metroData.StartSID, metroData.EndSID, metroData.DeductedFare)

	// 獲取數據後，將其保存到緩存中
	err = writeToCache(&metroData)
	if err != nil {
		log.Printf("Error writing to cache: %v", err)
		return nil, err
	}

	log.Printf("Successfully cached metro data")
	return &metroData, nil
}

func searchStations(query string) []Station {
	var matchingStations []Station
	for _, station := range stations {
		if strings.Contains(station.StationName, query) {
			matchingStations = append(matchingStations, station)
		}
	}
	return matchingStations
}

func calculateTotalFare(requests []FareRequest) (int, error) {
	totalFare := 0
	for i, req := range requests {
		log.Printf("Processing request %d: %s -> %s, RoundTrip: %v, Trips: %d",
			i+1, req.StartStationName, req.EndStationName, req.IsRoundTrip, req.Trips)

		startSID, err := getSIDByName(req.StartStationName)
		if err != nil {
			log.Printf("Error getting start station SID for '%s': %v", req.StartStationName, err)
			return 0, err
		}
		log.Printf("Start station SID: %s", startSID)

		endSID, err := getSIDByName(req.EndStationName)
		if err != nil {
			log.Printf("Error getting end station SID for '%s': %v", req.EndStationName, err)
			return 0, err
		}
		log.Printf("End station SID: %s", endSID)

		data, err := fetchMetroData(startSID, endSID)
		if err != nil {
			log.Printf("Error fetching metro data for %s -> %s: %v", startSID, endSID, err)
			return 0, err
		}

		fare, err := strconv.Atoi(data.DeductedFare)
		if err != nil {
			log.Printf("Error converting fare '%s' to int: %v", data.DeductedFare, err)
			return 0, err
		}

		if req.IsRoundTrip {
			fare *= 2
		}

		if req.Trips == 0 {
			req.Trips = 1 // 如果沒有帶「趟數」這個參數預設為1
		}
		totalFare += fare * req.Trips
		log.Printf("Request %d fare: %d, Total so far: %d", i+1, fare*req.Trips, totalFare)
	}

	return totalFare, nil
}

type CacheData []MetroData

func readFromCache(startSID, endSID string) (*MetroData, error) {
	cacheContents, err := ioutil.ReadFile("cache.json")
	if err != nil {
		// 如果文件不存在，返回 nil 表示没有缓存数据
		log.Printf("Cache file not found or error reading: %v", err)
		return nil, nil
	}

	// 如果文件为空，返回 nil 表示没有缓存数据
	if len(cacheContents) == 0 {
		log.Printf("Cache file is empty")
		return nil, nil
	}

	var cache CacheData
	err = json.Unmarshal(cacheContents, &cache)
	if err != nil {
		log.Printf("Error unmarshaling cache: %v", err)
		return nil, err
	}

	log.Printf("Searching cache with %d entries for %s -> %s", len(cache), startSID, endSID)
	for _, entry := range cache {
		if entry.StartSID == startSID && entry.EndSID == endSID {
			log.Printf("Found cached entry for %s -> %s", startSID, endSID)
			return &entry, nil
		}
	}

	log.Printf("No cached entry found for %s -> %s", startSID, endSID)
	return nil, nil
}

func writeToCache(data *MetroData) error {
	var cache CacheData

	cacheContents, err := ioutil.ReadFile("cache.json")
	if err != nil {
		// 如果文件不存在，创建一个空的缓存数组
		log.Printf("Cache file not found, creating new cache")
		cache = CacheData{}
	} else if len(cacheContents) == 0 {
		// 如果文件为空，创建一个空的缓存数组
		log.Printf("Cache file is empty, creating new cache")
		cache = CacheData{}
	} else {
		// 如果文件存在且不为空，解析现有数据
		err = json.Unmarshal(cacheContents, &cache)
		if err != nil {
			log.Printf("Error unmarshaling existing cache: %v", err)
			return err
		}
		log.Printf("Loaded existing cache with %d entries", len(cache))
	}

	cache = append(cache, *data)
	log.Printf("Added new entry to cache, total entries: %d", len(cache))

	newCacheContents, err := json.Marshal(cache)
	if err != nil {
		log.Printf("Error marshaling cache: %v", err)
		return err
	}

	err = ioutil.WriteFile("cache.json", newCacheContents, 0644)
	if err != nil {
		log.Printf("Error writing cache file: %v", err)
		return err
	}

	log.Printf("Successfully wrote cache file")
	return nil
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
