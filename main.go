package main

import (
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

	r.GET("/metrodata", func(c *gin.Context) {
		startName := c.DefaultQuery("startName", "")
		endName := c.DefaultQuery("endName", "")

		startsid := getSIDByName(startName)
		endsid := getSIDByName(endName)

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

func getSIDByName(name string) string {
	for _, station := range stations {
		if station.StationName == name {
			return station.StationSID
		}
	}
	return "" // 返回空字符串，如果没有找到
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

	client := &http.Client{}
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

	var metroData MetroData
	err = json.Unmarshal(body, &metroData)
	if err != nil {
		return nil, err
	}

	// 获取数据后，将其保存到缓存中
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
		startSID := getSIDByName(req.StartStationName)
		endSID := getSIDByName(req.EndStationName)

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
		totalFare += fare
	}

	return totalFare, nil
}

type CacheData []MetroData

func readFromCache(startSID, endSID string) (*MetroData, error) {
	cacheContents, err := ioutil.ReadFile("cache.json")
	if err != nil {
		return nil, err
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
	cacheContents, err := ioutil.ReadFile("cache.json")
	if err != nil {
		return err
	}

	var cache CacheData
	err = json.Unmarshal(cacheContents, &cache)
	if err != nil {
		return err
	}

	cache = append(cache, *data)

	newCacheContents, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return ioutil.WriteFile("cache.json", newCacheContents, 0644)
}
