package main

import (
	"cloud.google.com/go/translate"
	"context"
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

var size = 10
var expireDate = 10 * time.Minute
var cacheStories = make([]Story, 0)

type Story struct {
	By			string
	Descendants	int
	Id			int
	Score		int
	Time		int
	Title		string
	TitleCN		string
	Type		string
	Url			string
	Index		int
}

type IndexModel struct {
	Items	[]Story
}

func main() {
	//defer timeCost(time.Now())

	r := mux.NewRouter()
	r.HandleFunc("/", getIndex).Methods("GET")
	go delCache()
	log.Fatal(http.ListenAndServe(":2000", r))
}

func getIndex(w http.ResponseWriter, r *http.Request) {

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
			story := <- ch
			stories[story.Index] = story
		}

		cacheStories = stories
	}

	tmpl, err := template.ParseFiles("index.gohtml")
	if err != nil {
		log.Println(err)
	}

	err = tmpl.Execute(w, IndexModel{Items: cacheStories})
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
		return "", fmt.Errorf("Translate: %v", err)
	}
	if len(resp) == 0 {
		return "", fmt.Errorf("Translate returned empty response to text: %s", text)
	}
	return resp[0].Text, nil
}

func timeCost(start time.Time) {
	elapsed := time.Since(start)
	fmt.Printf("Done! It took %v seconds!\n", elapsed.Seconds())
}