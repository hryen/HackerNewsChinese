package main

import (
	"cloud.google.com/go/translate"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"golang.org/x/text/language"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// 建议缓存为3小时，20条 * 100个字符 * 一天8次 * 30天 = 4万，谷歌免费额度是50万
var size = 20
var expireDate = 1 * time.Hour

//go:embed index.gohtml
var index embed.FS

//go:embed static
var staticFiles embed.FS
var cacheStories = make([]Story, 0)
var cacheTimestamp time.Time

type Story struct {
	By          string
	Descendants int
	Id          int
	Score       int
	Time        int
	Title       string
	TitleCN     string
	Type        string
	Url         string
	Index       int
}

type IndexModel struct {
	Items          []Story
	CacheTimestamp time.Time
}

func main() {
	//defer timeCost(time.Now())

	r := mux.NewRouter()
	r.PathPrefix("/static/").Handler(http.StripPrefix("/", http.FileServer(http.FS(staticFiles))))
	r.HandleFunc("/", getIndex).Methods("GET")
	go delCache()
	log.Fatal(http.ListenAndServe("127.0.0.1:2001", r))
}

func getIndex(w http.ResponseWriter, _ *http.Request) {

	if len(cacheStories) == 0 {
		var arr, err = getTopStories()
		if err != nil {
			log.Fatal(err)
		}

		stories := make([]Story, size)
		ch := make(chan Story, size)

		for i := 0; i < size; i++ {
			go getStory(strconv.Itoa(arr[i]), ch, i)
		}

		for i := 0; i < size; i++ {
			story := <-ch
			stories[story.Index] = story
		}

		cacheStories = stories

		shanghai, _ := time.LoadLocation("Asia/Shanghai")
		cacheTimestamp = time.Now().In(shanghai)
	}

	tmpl, err := template.ParseFS(index, "index.gohtml")
	if err != nil {
		log.Println(err)
	}

	err = tmpl.Execute(w, IndexModel{Items: cacheStories, CacheTimestamp: cacheTimestamp})
	if err != nil {
		log.Println(err)
	}
}

func delCache() {
	for {
		cacheStories = make([]Story, 0)
		time.Sleep(expireDate)
	}
}

func getTopStories() ([]int, error) {
	fmt.Println("getTopStories...")
	url := "https://hacker-news.firebaseio.com/v0/topstories.json"
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var arr []int
	_ = json.Unmarshal(body, &arr)
	return arr, nil
}

func getStory(id string, ch chan Story, index int) {
	url := "https://hacker-news.firebaseio.com/v0/item/"
	resp, err := http.Get(url + id + ".json")
	if err != nil {
		ch <- Story{Title: "error"}
		return
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ch <- Story{Title: "error"}
		return
	}

	var story Story
	_ = json.Unmarshal(body, &story)
	titleCN, err := translateTextToChinese(story.Title)
	if err != nil {
		log.Println(err)
	}
	story.TitleCN = titleCN
	story.Index = index
	ch <- story
}

func translateTextToChinese(text string) (string, error) {
	ctx := context.Background()

	lang, err := language.Parse("zh-cn")
	if err != nil {
		return "", fmt.Errorf("language.Parse: %v", err)
	}

	client, err := translate.NewClient(ctx)
	if err != nil {
		return "", err
	}
	defer func(client *translate.Client) {
		_ = client.Close()
	}(client)

	resp, err := client.Translate(ctx, []string{text}, lang, nil)
	if err != nil {
		return "", fmt.Errorf("translate: %v", err)
	}
	if len(resp) == 0 {
		return "", fmt.Errorf("translate returned empty response to text: %s", text)
	}
	return resp[0].Text, nil
}

func timeCost(start time.Time) {
	elapsed := time.Since(start)
	fmt.Printf("Done! It took %v seconds!\n", elapsed.Seconds())
}
