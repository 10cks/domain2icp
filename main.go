package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

func main() {
	// 设置命令行参数
	domainsFilePath := flag.String("f", "", "Path to the file containing the list of domains.")
	outputFile := flag.String("o", "output.json", "Output file to write the JSON data to.")
	proxyServer := flag.String("p", "", "proxy server")
	flag.Parse()

	// 当没有参数的时候，打印help信息
	if len(os.Args) == 1 {
		flag.Usage()
		return
	}

	// 使用flag参数
	err := RemoveDuplicates(*domainsFilePath)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("Remove Duplication Run successfully.")
	}

	var httpClient *http.Client
	if *proxyServer != "" {
		proxyUrl, _ := url.Parse(*proxyServer)
		httpClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyUrl),
			},
		}
	} else {
		httpClient = &http.Client{}
	}

	// 打开包含域名的文件
	domainsFile, err := os.Open(*domainsFilePath)
	if err != nil {
		log.Fatalf("Failed to open the domains file: %v", err)
	}
	defer domainsFile.Close()

	// 准备输出文件
	file, err := os.Create(*outputFile)
	if err != nil {
		log.Fatalf("Failed to create the output file: %v", err)
	}
	defer file.Close()

	// 开始 JSON 数组
	// _, err = file.WriteString("[\n")
	if err != nil {
		log.Fatalf("Failed to write to output file: %v", err)
	}
	// 按行读取域名
	scanner := bufio.NewScanner(domainsFile)
	for scanner.Scan() {
		domain := scanner.Text()
		info, err := extractInformation(httpClient, domain)
		if err != nil {
			log.Printf("Error extracting information for domain %s: %v", domain, err)
			continue
		}

		// 生成用于控制台输出的 JSON
		prettyJSON, err := json.MarshalIndent(info, "", "    ")
		if err != nil {
			log.Printf("JSON marshaling error for domain %s: %v", domain, err)
			continue
		}

		// 输出到控制台
		fmt.Println(string(prettyJSON))

		// 生成用于文件存储的 JSON
		jsonData, err := json.Marshal(info)
		if err != nil {
			log.Printf("JSON marshaling error for domain %s: %v", domain, err)
			continue
		}

		// 将 JSON 数据写入输出文件
		_, err = file.Write(append(jsonData, '\n')) // 修改成fofaEX可处理的格式
		if err != nil {
			log.Fatalf("Failed to write to output file: %v", err)
		}
	}

	// 检查在扫描文件期间是否发生了错误
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	// 结束 JSON 数组
	// _, err = file.WriteString("\n]\n")
	if err != nil {
		log.Fatalf("Failed to write to output file: %v", err)
	}

	// 通知用户过程已完成
	fmt.Printf("Data saved to %s\n", *outputFile)
}

// Information represents the extracted information
type Information struct {
	Domain            string `json:"domain"`
	CompanyName       string `json:"company_name"`
	CompanyType       string `json:"company_type"`
	RegisteredCapital string `json:"registered_capital"`
	RegistrationTime  string `json:"registration_time"`
	RegisteredAddress string `json:"registered_address"`
	ICPPermit         string `json:"icp_permit"`
}

func extractInformation(client *http.Client, domain string) (*Information, error) {
	url := fmt.Sprintf("https://icp.chinaz.com/%s", domain)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET error: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body error: %w", err)
	}

	infoPatterns := map[string]*regexp.Regexp{
		"CompanyName":       regexp.MustCompile(`企业名称</td>\s*<td[^>]*>([^<]+)</td>`),
		"CompanyType":       regexp.MustCompile(`公司类型</td>\s*<td[^>]*>([^<]+)</td>`),
		"RegisteredCapital": regexp.MustCompile(`注册资本</td>\s*<td[^>]*>([^<]+)</td>`),
		"RegistrationTime":  regexp.MustCompile(`注册时间</td>\s*<td[^>]*>([^<]+)</td>`),
		"RegisteredAddress": regexp.MustCompile(`注册地址</td>\s*<td[^>]*>\s*<div[^>]*>([^<]+)</div>`),
	}

	info := Information{}
	info.Domain = domain
	for key, pattern := range infoPatterns {
		matches := pattern.FindSubmatch(body)
		if matches != nil && len(matches) > 1 {
			switch key {
			case "CompanyName":
				info.CompanyName = string(matches[1])
			case "CompanyType":
				info.CompanyType = string(matches[1])
			case "RegisteredCapital":
				info.RegisteredCapital = string(matches[1])
			case "RegistrationTime":
				info.RegistrationTime = string(matches[1])
			case "RegisteredAddress":
				info.RegisteredAddress = string(matches[1])
			}
		}
	}

	// 用于解析POST请求返回的数据的结构体
	type Response struct {
		Code int    `json:"code"`
		Data string `json:"data"`
		Msg  string `json:"msg"`
	}

	// 创建请求体数据
	postData := map[string]string{"keyword": domain}
	postBytes, err := json.Marshal(postData)
	if err != nil {
		return nil, err
	}

	postReader := bytes.NewReader(postBytes)

	// 创建POST请求
	req, err := http.NewRequest("POST", "https://icp.chinaz.com/index/api/queryPermit", postReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	// 设置其他的HTTP头部信息，比如Cookie、User-Agent

	// 发起请求，获取返回的数据
	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// 解析返回的JSON数据
	var res Response
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, err
	}

	info.ICPPermit = res.Data

	return &info, nil
}

func RemoveDuplicates(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	outfile, err := os.Create("outfile.txt")
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(outfile)

	for scanner.Scan() {
		line := scanner.Text()
		// 检查是否为空行
		if strings.TrimSpace(line) != "" && !seen[line] {
			seen[line] = true
			_, err := writer.WriteString(line + "\n")
			if err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	err = writer.Flush()
	if err != nil {
		return err
	}

	err = outfile.Close()
	if err != nil {
		return err
	}

	// 显式地关闭旧文件
	err = file.Close()
	if err != nil {
		return err
	}

	err = os.Remove(filePath)
	if err != nil {
		return err
	}

	err = os.Rename("outfile.txt", filePath)

	return err
}
