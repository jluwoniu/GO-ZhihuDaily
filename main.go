package main

import (
	"database/sql"
	"fmt"
	"github.com/bitly/go-simplejson"
	"github.com/codegangsta/martini"
	"github.com/codegangsta/martini-contrib/render"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// 20060102 15:04:05

type UsedData struct {
	Date      string
	MainPages []MainPage
}

type MainPage struct {
	Id    int
	Title string
}

type FinalData struct {
	Useddata []UsedData
	Pagemark []int
}

func zhihuDailyJson(str string) UsedData {

	sj, _ := simplejson.NewJson([]byte(str))

	news, _ := sj.Get("news").Array()
	tmp, _ := time.Parse("20060102", sj.Get("date").MustString())
	date := tmp.Format("2006.01.02 Monday")

	var mainpages []MainPage

	os.Mkdir(PIC, 755)

	for _, a := range news {
		m := a.(map[string]interface{})
		title := m["title"].(string)
		url := m["url"].(string)

		id := atoi(url[strings.LastIndexAny(url, "/")+1:])

		mainpages = append(mainpages, MainPage{id, title})
	}

	return UsedData{Date: date, MainPages: mainpages}
}

func Exist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}

func renderPages(days int) map[int]FinalData {
	pages := make(map[int]FinalData)
	var pagemark []int
	date := time.Now()

	memoreyCache := QueryData()

	for i := 1; i <= len(memoreyCache)/days; i += 1 {
		pagemark = append(pagemark, i)
	}

	for i := 1; i <= len(memoreyCache)/days; i += 1 {

		var finaldata FinalData
		var useddata []UsedData

		if i == 1 {
			useddata = append(useddata, zhihuDailyJson(todayData()))
		}

		for j := 0; j < days; j++ {
			key := date.Format("20060102")

			data, ok := memoreyCache[atoi(key)]
			if !ok {
				data = getBeforeData(key)
			}

			useddata = append(useddata, zhihuDailyJson(data))
			date = date.AddDate(0, 0, -1)
		}
		finaldata.Useddata = useddata
		finaldata.Pagemark = pagemark
		pages[i] = finaldata
	}

	return pages
}

func atoi(s string) int {
	dateInt, _ := strconv.Atoi(s)
	return dateInt
}

func autoUpdate() map[int]FinalData {

	// init
	days := 4
	pages := renderPages(days)

	ticker := time.NewTicker(time.Hour) // update every per hour
	go func() {
		for t := range ticker.C {
			fmt.Println("renderPages at ", t)
			pages = renderPages(days)
		}
	}()

	return pages
}

var PIC = "static/pic/"

func main() {

	pages := autoUpdate()

	m := martini.Classic()
	m.Use(martini.Static("static"))
	m.Use(render.Renderer())

	m.Get("/", func(r render.Render) {

		r.HTML(200, "content", []interface{}{pages[1]})
	})

	m.Get("/page/:id", func(params martini.Params, r render.Render) {

		id := atoi(params["id"])
		r.HTML(200, "content", []interface{}{pages[id]})
	})

	http.ListenAndServe("0.0.0.0:8000", m)
	m.Run()
}

func download(path string, url string) {
	fmt.Println(url)

	resp, err := http.Get(url)
	checkErr(err)

	defer resp.Body.Close()

	id := url[strings.LastIndexAny(url, "/")+1:]

	file, err := os.Create(path + id)
	checkErr(err)

	io.Copy(file, resp.Body)
}

// -------------------DB----------------------
func getNews(id string) string {
	eachnews := QueryEachNewsData(atoi(id))

	if eachnews == "" {
		url := "http://daily.zhihu.com/api/1.2/news/" + id
		eachnews := getData(url)
		writeToNewsIdDB(atoi(id), eachnews)
		//fmt.Println(eachnews)
	}

	sj, err := simplejson.NewJson([]byte(eachnews))
	checkErr(err)
	body := sj.Get("body").MustString()

	return body
}

func getData(url string) string {
	resp, err := http.Get(url)
	checkErr(err)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	return string(body)
}

func getBeforeData(date string) string {
	url := "http://news.at.zhihu.com/api/1.2/news/before/" + date
	data := getData(url)

	writeToDB(atoi(date), data)

	return data
}

func todayData() string {
	url := "http://news.at.zhihu.com/api/1.2/news/latest"

	return getData(url)
}

func QueryData() map[int]string {

	memoryCache := make(map[int]string)

	db, err := sql.Open("sqlite3", "./main.db")
	checkErr(err)

	rows, err := db.Query("SELECT * FROM datainfo")
	checkErr(err)

	db.Close()

	for rows.Next() {
		var date int
		var data string
		err = rows.Scan(&date, &data)
		memoryCache[date] = data
	}

	return memoryCache
}

func QueryEachNewsData(id int) string {

	db, err := sql.Open("sqlite3", "./main.db")
	checkErr(err)

	rows, err := db.Query("SELECT * FROM eachnews")
	checkErr(err)

	db.Close()

	for rows.Next() {
		var index int
		var body string
		err = rows.Scan(&index, &body)
		if index == id {
			return body
		}
	}

	return ""
}

func writeToDB(date int, data string) {

	db, err := sql.Open("sqlite3", "./main.db")
	checkErr(err)
	//插入数据
	stmt, err := db.Prepare("INSERT INTO datainfo(date, data) values(?,?)")
	checkErr(err)

	res, err := stmt.Exec(date, data)
	checkErr(err)

	id, err := res.LastInsertId()
	checkErr(err)

	fmt.Println(id)

	db.Close()
}

func writeToNewsIdDB(id int, body string) {

	db, err := sql.Open("sqlite3", "./main.db")
	checkErr(err)
	//插入数据
	stmt, err := db.Prepare("INSERT INTO eachnews(id, body) values(?,?)")
	checkErr(err)

	res, err := stmt.Exec(id, body)
	checkErr(err)

	index, err := res.LastInsertId()
	checkErr(err)

	fmt.Println(index)

	db.Close()
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
