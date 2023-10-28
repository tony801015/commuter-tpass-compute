# 通勤族TPASS計算

目前只先實作捷運版，未來之後有空再加上Ubike和公車和火車  
或許也會提供UI介面以便操作(當然也歡迎大家一起實作🤣)

# 用法
- 計算起點到和目的地
```
curl --location 'http://localhost:8080/metrodata?startName=後山埤&endName=土城'
```
回傳格式：
```
{
    "StartSID": "095",
    "EndSID": "078",
    "StartStationName": "後山埤",
    "EndStationName": "土城",
    "DeductedFare": "45",
    "Discount60": "27",
    "Discount40": "18",
    "Lang": "tw"
}
```
- 查詢MRT的SID和名稱
```
curl --location 'http://localhost:8080/searchstations?query=南'
```
回傳格式：
```
[
    {
        "StationSID": "009",
        "StationName": "南京復興"
    },
    {
        "StationSID": "022",
        "StationName": "劍南路"
    },
    {
        "StationSID": "030",
        "StationName": "南港軟體園區"
    },
    {
        "StationSID": "031",
        "StationName": "南港展覽館"
    },
    {
        "StationSID": "043",
        "StationName": "小南門"
    },
    {
        "StationSID": "132",
        "StationName": "松江南京"
    },
    {
        "StationSID": "009",
        "StationName": "南京復興"
    },
    {
        "StationSID": "110",
        "StationName": "南京三民"
    },
    {
        "StationSID": "048",
        "StationName": "南勢角"
    },
    {
        "StationSID": "132",
        "StationName": "松江南京"
    },
    {
        "StationSID": "097",
        "StationName": "南港"
    },
    {
        "StationSID": "031",
        "StationName": "南港展覽館"
    }
]
```
- 計算多點移動費用
```
curl --location 'http://localhost:8080/calculatefare' \
--header 'Content-Type: application/json' \
--data '[
    {
        "startStationName": "後山埤",
        "endStationName": "信義安和",
        "isRoundTrip": false
    },
    {
        "startStationName": "信義安和",
        "endStationName": "北投",
        "isRoundTrip": false
    },
    {
        "startStationName": "北投",
        "endStationName": "松江南京",
        "isRoundTrip": false
    },
    {
        "startStationName": "松江南京",
        "endStationName": "後山埤",
        "isRoundTrip": false
    }
]'
```
回傳格式：
```
{
    "totalFare": 125
}
```