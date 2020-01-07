package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/eyuyan-dev/go-common/ext"

	"github.com/eyuyan-dev/go-common/utils"

	"github.com/eyuyan-dev/go-common/request"

	"gopkg.in/cheggaaa/pb.v1"
)

const (
	//下载线程
	ThreadNumber = 5
	//失败重试次数
	RetryTimes = 3
)

//进度
func progressBar(size int64) *pb.ProgressBar {
	bar := pb.New64(size).SetUnits(pb.U_BYTES).SetRefreshRate(time.Millisecond * 10)
	bar.ShowSpeed = true
	bar.ShowFinalTime = true
	bar.ShowCounters = true
	bar.ShowElapsedTime = true
	bar.ShowBar = true
	bar.ShowTimeLeft = true
	bar.SetMaxWidth(1000)
	return bar
}

func writeChuckFile(url string, file *os.File, headers map[string]string, bar *pb.ProgressBar) (int64, error) {

	client := &http.Client{}

	req, err := http.NewRequest(http.MethodGet, url, nil)

	if err != nil {
		return 0, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var (
		res          *http.Response
		requestError error
	)

	for i := 0; ; i++ {
		res, requestError = client.Do(req)
		if requestError == nil && res.StatusCode < 400 {
			break
		} else if i+1 >= RetryTimes {
			var err error
			if requestError != nil {
				err = fmt.Errorf("request error: %v", requestError)
			} else {
				err = fmt.Errorf("%s request error: HTTP %d", url, res.StatusCode)
			}
			return 0, err
		}
		time.Sleep(1 * time.Second)
	}

	defer res.Body.Close()

	writer := io.MultiWriter(file, bar)
	// Note that io.Copy reads 32kb(maximum) from input and writes them to output, then repeats.
	// So don't worry about memory.
	written, copyErr := io.Copy(writer, res.Body)
	if copyErr != nil && copyErr != io.EOF {
		return written, fmt.Errorf("file copy error: %s", copyErr)
	}
	return written, nil
}

func Download(downUrl, refer, filePath string, chunkSizeMB int) (bool, error) {

	//获取网络文件大小
	remainingSize, err := request.Size(downUrl, nil)

	if err != nil {
		fmt.Println(err.Error())
		return false, err
	}

	//判断本地文件是否和网络文件一样大
	size, exist, err := ext.FileSize(filePath)
	if exist && err == nil && size == remainingSize {
		return true, nil
	}

	//进度
	bar := progressBar(remainingSize)
	bar.Start()

	var i int64 = 1

	wgp := utils.NewWaitGroupPool(ThreadNumber)
	errs := make([]error, 0)
	lock := sync.Mutex{}

	//根据块大小计算分块数量
	var start, end, chunkSize int64
	chunkSize = int64(chunkSizeMB) * 1024 * 1024
	chunk := remainingSize / chunkSize
	if remainingSize%chunkSize != 0 {
		chunk++
	}

	//按块下载
	for ; i <= chunk; i++ {
		wgp.Add()
		end = start + chunkSize - 1
		if end > remainingSize {
			end = remainingSize
		}
		go func(_url, _refer, _filePath string, index, _start, _end, _chunk, _chunkSize, remainingSize int64, _wgp *utils.WaitGroupPool, _bar *pb.ProgressBar) {
			defer _wgp.Done()
			headers := map[string]string{
				"Referer": _refer,
			}
			tempFilePath := fmt.Sprintf("%s.download%d", _filePath, index)
			tempFileSize, _, err := ext.FileSize(tempFilePath)
			if err != nil {
				lock.Lock()
				errs = append(errs, err)
				lock.Unlock()
				return
			}

			var (
				file      *os.File
				fileError error
			)

			if tempFileSize > 0 {
				//完整的或者最后一块,就直接返回     判断是否位最后一块且大小相等
				if tempFileSize == _chunkSize || (remainingSize%chunkSize == tempFileSize && index == _chunk) {
					bar.Add64(tempFileSize)
					return
				}

				//如果续传块的话程序异常中断的时候有问题,那就直接整块重新下载

				/*				_start += tempFileSize + 1
								file, fileError = os.OpenFile(tempFilePath, os.O_APPEND|os.O_WRONLY, 0644)
								bar.Add64(tempFileSize)*/
				file, fileError = os.Create(tempFilePath)
			} else {
				file, fileError = os.Create(tempFilePath)
			}

			if fileError != nil {
				lock.Lock()
				errs = append(errs, err)
				lock.Unlock()
				return
			}

			defer file.Close()

			headers["Range"] = fmt.Sprintf("bytes=%d-%d", _start, _end)

			temp := _start

			for i := 0; ; i++ {
				written, err := writeChuckFile(_url, file, headers, _bar)
				if err == nil {
					break
				} else if i+1 >= RetryTimes {
					lock.Lock()
					errs = append(errs, err)
					lock.Unlock()
					return
				}
				temp += written
				headers["Range"] = fmt.Sprintf("bytes=%d-%d", temp, _end)
				time.Sleep(1 * time.Second)
			}

		}(downUrl, refer, filePath, i, start, end, chunk, chunkSize, remainingSize, wgp, bar)
		if end == remainingSize {
			break
		}
		start = end + 1
	}

	wgp.Wait()

	bar.Finish()

	for _, v := range errs {
		fmt.Println(v.Error())
	}

	//没有错误,就合并所有的块
	if len(errs) == 0 {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return false, err
		}
		defer f.Close()

		i = 1

		for ; i <= chunk; i++ {
			fileName := fmt.Sprintf("%s.download%d", filePath, i)
			fh, err := os.Open(fileName)
			if err != nil {
				return false, err
			}
			_, err = io.Copy(f, fh)
			if err != nil {
				return false, err
			}
			fh.Close()
			os.Remove(fileName)
		}
	}

	return true, nil
}
