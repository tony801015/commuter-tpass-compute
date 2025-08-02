package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
		var requests []FareRequest
		if err := c.BindJSON(&requests); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request data",
			})
			return
		}

		totalFare, err := calculateTotalFare(requests)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

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
	// 先尝试从缓存中读取数据
	data, err := readFromCache(startSID, endSID)
	if err != nil {
		return nil, err
	}

	if data != nil {
		return data, nil // 如果缓存中有数据，直接返回缓存数据
	}

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
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// 檢查 HTTP 狀態碼
	if res.StatusCode != http.StatusOK {
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
		return nil, fmt.Errorf("failed to parse JSON response: %v. Response preview: %s", err, bodyPreview)
	}

	// 獲取數據後，將其保存到緩存中
	err = writeToCache(&metroData)
	if err != nil {
		return nil, err
	}

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
	for _, req := range requests {
		startSID, err := getSIDByName(req.StartStationName)
		if err != nil {
			return 0, err
		}
		endSID, err := getSIDByName(req.EndStationName)
		if err != nil {
			return 0, err
		}

		data, err := fetchMetroData(startSID, endSID)
		if err != nil {
			return 0, err
		}

		fare, err := strconv.Atoi(data.DeductedFare)
		if err != nil {
			return 0, err
		}

		if req.IsRoundTrip {
			fare *= 2
		}

		if req.Trips == 0 {
			req.Trips = 1 // 如果沒有帶「趟數」這個參數預設為1
		}
		totalFare += fare * req.Trips
	}

	return totalFare, nil
}

type CacheData []MetroData

func readFromCache(startSID, endSID string) (*MetroData, error) {
	cacheContents, err := ioutil.ReadFile("cache.json")
	if err != nil {
		// 如果文件不存在，返回 nil 表示没有缓存数据
		return nil, nil
	}

	// 如果文件为空，返回 nil 表示没有缓存数据
	if len(cacheContents) == 0 {
		return nil, nil
	}

	var cache CacheData
	err = json.Unmarshal(cacheContents, &cache)
	if err != nil {
		return nil, err
	}

	for _, entry := range cache {
		if entry.StartSID == startSID && entry.EndSID == endSID {
			return &entry, nil
		}
	}

	return nil, nil
}

func writeToCache(data *MetroData) error {
	var cache CacheData

	cacheContents, err := ioutil.ReadFile("cache.json")
	if err != nil {
		// 如果文件不存在，创建一个空的缓存数组
		cache = CacheData{}
	} else if len(cacheContents) == 0 {
		// 如果文件为空，创建一个空的缓存数组
		cache = CacheData{}
	} else {
		// 如果文件存在且不为空，解析现有数据
		err = json.Unmarshal(cacheContents, &cache)
		if err != nil {
			return err
		}
	}

	cache = append(cache, *data)

	newCacheContents, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return ioutil.WriteFile("cache.json", newCacheContents, 0644)
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
