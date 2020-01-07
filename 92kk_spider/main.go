package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eyuyan-dev/go-common/ext"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/extensions"
)

func main() {

	c := colly.NewCollector(func(collector *colly.Collector) {
		collector.Async = true
		extensions.RandomUserAgent(collector) // 随机UA
		extensions.Referer(collector)         // referer自动填写
	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL)
	})

	c.OnError(func(_ *colly.Response, err error) {
		fmt.Println("Something went wrong:", err)
	})

	c.OnResponse(func(r *colly.Response) {
		if r.Request.URL.Path == "/index.php/ajax/dance_show" {
			videos, err := UnmarshalVideos(r.Body)
			if err == nil {
				for _, v := range videos {
					fileUrl, _ := url.Parse("http://listen.9sing.cn" + v.FilePath)
					fmt.Println(v.ClassName, v.DanceName, fileUrl.String())
					videoList = append(videoList, v)
				}
			}
		}
		fmt.Println("Visited", r.Request.URL)
	})

	//列表页面,爬取所有ID
	c.OnHTML("ul[class='share_list']", func(e *colly.HTMLElement) {
		var dIDs []string
		e.ForEach("li>div>span>input", func(i int, element *colly.HTMLElement) {
			id := element.Attr("value")
			if _, ok := strconv.Atoi(id); ok == nil {
				dIDs = append(dIDs, id)
			}
		})
		idsStr := strings.Join(dIDs, ",")
		c.Visit("http://www.92kk.com/index.php/ajax/dance_show?did=" + idsStr)
	})

	//下一页
	c.OnHTML("a[title='后一页']", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		c.Visit(e.Request.AbsoluteURL(link))
	})

	c.OnScraped(func(r *colly.Response) {
		fmt.Println("Finished", r.Request.URL)
	})

	c.Visit("http://www.92kk.com/dance/lists-id-16-1.html")

	c.Wait()

	_dir := "videos"
	dirExist := ext.PathIsExist(_dir)
	if !dirExist {
		os.Mkdir(_dir, os.ModePerm)
	}

	for _, v := range videoList {
		_, fileName := filepath.Split(v.FilePath)
		filePath := _dir + "/" + fileName
		fileUrl, _ := url.Parse("http://listen.9sing.cn" + v.FilePath)
		fmt.Println("开始下载:", v.DanceName)
		Download(fileUrl.String(), "http://www.92kk.com/dance/lists-id-16-1.html", filePath, 5)
	}
}

type Videos map[string]VideosValue

func UnmarshalVideos(data []byte) (Videos, error) {
	var r Videos
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *Videos) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type VideosValue struct {
	DanceName string `json:"dance_name"`
	ClassID   string `json:"class_id"`
	FilePath  string `json:"file_path"`
	ClassName string `json:"className"`
}

var videoList []VideosValue
