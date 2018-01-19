package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/pb"
)

var ps []*os.Process //保存所有的子进程
var mux sync.Mutex   //子进程切片锁

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

func downFile(url string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	// 解析/后的文件名字
	urlMap := strings.Split(url, "/")
	fileName := urlMap[len(urlMap)-1]

	// 解析带? = fileName 的文件名字
	if strings.Contains(fileName, "=") {
		splitName := strings.Split(fileName, "=")
		fileName = splitName[len(splitName)-1]
	}

	// 判断get url的状态码, StatusOK = 200
	if resp.StatusCode == http.StatusOK {
		log.Printf("[INFO] 正在下载: [%s]", fileName)
		fmt.Print("\n")

		downFile, err := os.Create(fileName)
		if err != nil {
			log.Fatal(err)
		}
		// 不要忘记关闭打开的文件.
		defer downFile.Close()

		// 获取下载文件的大小
		i, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
		sourceSiz := int64(i)
		source := resp.Body

		// 创建一个进度条
		bar := pb.New(int(sourceSiz)).SetUnits(pb.U_BYTES).SetRefreshRate(time.Millisecond * 10)

		// 显示下载速度
		bar.ShowSpeed = true

		// 显示剩余时间
		bar.ShowTimeLeft = true

		// 显示完成时间
		bar.ShowFinalTime = true

		bar.SetMaxWidth(80)

		bar.Start()

		writer := io.MultiWriter(downFile, bar)
		io.Copy(writer, source)
		bar.Finish()

		fmt.Print("\n")
		log.Printf("[INFO] [%s]下载成功.", fileName)
	} else {
		fmt.Print("\n")
		log.Printf("[ERROR] [%s]下载失败,%s.", fileName, resp.Status)
	}
}

func runServer(php string, args []string) {
	env := os.Environ()
	attr := &os.ProcAttr{
		Env:   env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}, //其他变量如果不清楚可以不设定
	}
	//	p, err := os.StartProcess("C:\\WINDOWS\\system32\\ping.exe", []string{"C:\\WINDOWS\\system32\\ping.exe", "www.noxue.com"}, attr) //vim 打开tmp.txt文件
	p, err := os.StartProcess(php, args, attr)
	if err != nil {
		fmt.Println(err)
	}

	mux.Lock()
	defer mux.Unlock()

	ps = append(ps, p)
}

func readConfig(path string) string {
	fi, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer fi.Close()
	fd, err := ioutil.ReadAll(fi)
	return string(fd)
}

/**
 * 判断文件是否存在  存在返回 true 不存在返回false
 */
func checkFile(filename string) bool {
	var exist = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}
	return exist
}

func CopyFile(dstName, srcName string) (written int64, err error) {
	src, err := os.Open(srcName)
	if err != nil {
		return
	}
	defer src.Close()

	dst, err := os.OpenFile(dstName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer dst.Close()

	return io.Copy(dst, src)
}

func main() {

	if !checkFile("php") {
		fmt.Println("开始下载PHP，根据个人网速大概将花费几分钟时间，请您耐心等待...")
		downFile("http://static.noxue.com/php-7.1.13-nts-Win32-VC14-x64.zip")

		unzip("php-7.1.13-nts-Win32-VC14-x64.zip", "./php/")
		os.Remove("./php-7.1.13-nts-Win32-VC14-x64.zip")
		CopyFile("./php/php.ini", "./php/php.ini-development")
	}

	php := "./php/php.exe"
	ConfigFile := "config.txt"

	var f1 *os.File
	if !checkFile("www") {
		os.Mkdir("www", 755)
		os.Mkdir("www/default", 755)
		f1, _ = os.Create("www/default/index.php") //创建文件
		io.WriteString(f1, "<?php\nphpinfo();")    //写入文件(字符串)
		f1.Close()
	}

	var f *os.File
	if !checkFile(ConfigFile) { //如果文件不存在

		f, _ = os.Create(ConfigFile)                                                                  //创建文件
		io.WriteString(f, "#配置方法\r\n#主机名:端口号  网站根目录路径\r\n#以#开头的为注释\r\n127.0.0.1:10010 ./www/default") //写入文件(字符串)
		f.Close()
	}

	var lastModTime int64
	//	if fileInfo, err := os.Stat("doc.go"); err == nil {
	//		lastModTime = fileInfo.ModTime().Unix()
	//	}

	for {
		if fileInfo, err := os.Stat(ConfigFile); err == nil && fileInfo.ModTime().Unix() > lastModTime {

			//已被修改，先结束以前的子进程
			for _, v := range ps {
				v.Kill()    //结束进程
				v.Release() //释放资源
			}

			// 重新读取修改后的配置文件，启动服务器
			str := readConfig(ConfigFile)
			strs := strings.Split(str, "\n")
			for _, v := range strs {
				line := strings.TrimSpace(v)
				if len(line) > 0 && line[0] != '#' {

					re, _ := regexp.Compile("\\s+")
					lines := re.Split(line, 2)
					if len(lines) == 2 {
						go runServer(php, []string{"php.exe", fmt.Sprint("-S", lines[0]), fmt.Sprint("-t", lines[1])})

					}
				}
			}

			//等待一秒，让提示信息显示在最后。
			time.Sleep(time.Second * 1)
			if lastModTime == 0 {
				fmt.Println("\r\n\r\n-------------php粉，最简洁的php开发环境 v1.0.0----------------\n-------------一分钟搭建php开发环境\n-------------phpfen.com----------------\n-------------php粉-----------------\n服务器启动成功.....\n默认地址：127.0.0.1:10010")

			} else {
				fmt.Println("\r\n\r\n-------------------\r\n配置文件重新加载完毕！\r\n\r\n-------------------\r\n")
			}

			lastModTime = fileInfo.ModTime().Unix()

		}
		time.Sleep(time.Second * 1)
	}

}
