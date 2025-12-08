package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"main/ai"
	"main/workerPool"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

var (
	mu      sync.Mutex
	scanner = bufio.NewScanner(os.Stdin)
	score   int
	win     int
	wg      sync.WaitGroup
)

func main() {
	defer bufio.NewReader(os.Stdin).ReadBytes('\n') // system("pause");
	fmt.Println("============================================================================================")
	fmt.Println("  CQUPT-C语言自动刷题系统")
	fmt.Println("  原作者：MaoMeng")
	fmt.Println("  原作者Github网址：https://github.com/MaoMengww")
	fmt.Println("  原项目地址：https://github.com/MaoMengww/CQUPT-auto-c-train")
	fmt.Println("")
	fmt.Println("  未经许可，严禁复制、修改、分发或用于商业用途")
	fmt.Println("  此脚本仅用于代码学习和研究使用，不得用于违规违纪行为，作者不对软件使用后果承担任何责任")
	fmt.Println()
	fmt.Println("====================================CQUPT-C语言刷题脚本=====================================")
	err := InitEnv(false)
	if err != nil {
		log.Fatalf("InitEnv err: %v", err)
	}
	err = ai.InitAI()
	if err != nil {
		log.Fatalf("ai初始化失败")
	}
	fmt.Println("请输入你的学号")
	scanner.Scan()
	username := scanner.Text()
	fmt.Println("请输入你的密码")
	scanner.Scan()
	password := scanner.Text()
	fmt.Println("请一字不差的输入你想刷的章节")
	scanner.Scan()
	choi := scanner.Text()
	fmt.Println("输入你想刷的题目数量")
	scanner.Scan()
	numStr := scanner.Text()
	num, err := strconv.Atoi(numStr)
	wg.Add(num)
	// 协程池工作
	fmt.Println("输入并发量(同一时间最大刷题数量,不输入则默认为12)")
	scanner.Scan()
	workCount := scanner.Text()
	workNum, err := strconv.Atoi(workCount)
	if workNum == 0 {
		workNum = 12
	}
	fmt.Println("--------------------------")
	pool := workerPool.NewWorkerPool(workNum, num+1)
	defer pool.Close()
	fmt.Printf("开始提交%v个任务\n", num)
	//协程池分发任务
	for i := range num {
		taskID := i
		taskFunc := StartWork(choi, username, password, i, &score)

		err := pool.Produce(taskFunc)
		if err != nil {
			log.Printf("提交任务 %v 失败：%v", taskID, err)
		}
		fmt.Println("已提交任务 ", taskID+1)
	}
	fmt.Println("--------------------------")
	wg.Wait()
	fmt.Println("最终得分为", score, "分,成功答题", win, "个")
	fmt.Println("请按回车键退出...")
}

// StartWork 封装即将执行的任务
func StartWork(choi, username, password string, i int, score *int) workerPool.TaskFunc {
	return func() {
		GetScore(choi, username, password, i, score, &win)
	}
}

func GetScore(choi, username, password string, i int, score *int, win *int) {
	defer wg.Done()
	taskID := i + 1
	fmt.Printf("  --任务%v：开始执行 \n", taskID)
	var scoreStr string
	//创建chrome实例
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.ExecPath(`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`),
	)
	alloctx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx1, _ := chromedp.NewContext(alloctx)
	ctx, cancel := context.WithTimeout(ctx1, 2*time.Minute)
	defer cancel()

	//要刷的章节
	selector := fmt.Sprintf(`//span[contains(text(), "%s")]`, choi)

	err := chromedp.Run(ctx,
		chromedp.Navigate("http://172.22.181.82/train/#/login"),
		//输入学号
		chromedp.WaitVisible(`input[name="username"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="username"]`, username, chromedp.ByQuery),
		//输入密码
		chromedp.WaitVisible(`input[name="password"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, password, chromedp.ByQuery),
		//点击登录
		chromedp.WaitVisible(`button[color="primary"]:not([disabled])`, chromedp.ByQuery),
		chromedp.Click(`button[color="primary"]`, chromedp.ByQuery),
		//进入开始练习页面
		chromedp.WaitVisible(`button[routerlink="choose-mode"]`, chromedp.ByQuery),
		chromedp.Click(`button[routerlink="choose-mode"]`, chromedp.ByQuery),
		//点击要刷的习题
		chromedp.WaitVisible(selector, chromedp.BySearch),
		chromedp.Click(selector, chromedp.BySearch),
		//点击开始
		chromedp.WaitVisible(`div#group button`, chromedp.ByQuery),
		chromedp.Click(`div#group button`, chromedp.ByQuery),
	)
	if err != nil {
		log.Printf("任务%v：登录或导航失败/超时: %v\n", taskID, err)
		return
	}
	fmt.Printf("任务%v：登陆成功\n", taskID)
	//验证是否刷完
	checkCtx, checkCancel := context.WithTimeout(ctx, 1*time.Second)
	defer checkCancel()
	fmt.Printf("任务%v：正在检查完成情况...\n", taskID)
	err = chromedp.Run(checkCtx,
		chromedp.WaitVisible(`//div[contains(text(), "你已经完成了")]`, chromedp.BySearch),
	)
	if err == nil {
		log.Printf("任务%v：你已经完成该标签下的所有题目\n", taskID)
		log.Printf("任务%v：程序退出\n", taskID)
		time.Sleep(20 * time.Second)

		return
	}
	if err == context.DeadlineExceeded {
		var question string
		//获取题目
		err := chromedp.Run(ctx,
			chromedp.Text(`.q-content`, &question, chromedp.ByQuery),
		)
		if err != nil {
			log.Printf("任务%v：获取题目失败\n", taskID)
			return
		}
		fmt.Printf("  --任务%v：解题中...\n", taskID)
		answer, err := ai.Answer(question)
		if err != nil {
			log.Printf("任务%v：答案生成失败:err: %v\n", taskID, err)
			return
		}
		codeBytes, _ := json.Marshal(answer)
		fillCodeJS := fmt.Sprintf(`monaco.editor.getEditors()[0].setValue(%s)`, string(codeBytes))

		err = chromedp.Run(ctx,
			//填充答案
			chromedp.WaitVisible(`.monaco-editor`, chromedp.ByQuery),
			chromedp.Evaluate(fillCodeJS, nil),
			//点击提交
			chromedp.WaitVisible(`//button[contains(., "提交")]`, chromedp.BySearch),
			chromedp.Click(`//button[contains(., "提交")]`, chromedp.BySearch),
			//点击确认
			chromedp.WaitVisible(`//button[contains(., "确定")]`, chromedp.BySearch),
			chromedp.Click(`//button[contains(., "确定")]`, chromedp.BySearch),
			//获取得分
			chromedp.WaitVisible(`tr[role="row"] td.mat-column-score`),
			chromedp.Text(`tr[role="row"] td.mat-column-score`, &scoreStr),
		)
		if err != nil {
			log.Printf("任务%v：填充代码或提交失败: %v\n", taskID, err)
			return
		}

		//计算单个任务得分
		fmt.Printf("  --任务%v：已解答,得分为%v分\n", taskID, scoreStr)
		scoreInt, _ := strconv.Atoi(scoreStr)
		if scoreInt == 0 {
			fmt.Println("已完成任务 ", taskID)
			return
		}
		mu.Lock()
		*score += scoreInt
		mu.Unlock()
	} else {
		//出错
		log.Printf("任务%v：检测过程发生未知错误: %v\n", taskID, err)

		return
	}
	fmt.Println("已完成任务 ", taskID)
	mu.Lock()
	*win++
	mu.Unlock()
}

func InitEnv(Rewrite bool) error {
	envFile := ".env"

	// 检查 .env 文件是否存在
	if _, err := os.Stat(envFile); os.IsNotExist(err) || !isEnvFileValid(envFile) || Rewrite {
		if !Rewrite {
			fmt.Println("检测到缺少环境配置文件或环境配置文件已损坏，正在启动环境配置程序...")
		}

		reader := bufio.NewReader(os.Stdin)

		// 获取 ARK_API_KEY
		var apiKey string
		for {
			fmt.Print("请输入大模型 API 密钥: ")
			input, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(input)

			if apiKey == "" {
				fmt.Println("API 密钥不能为空")
				continue
			}

			// 验证格式
			if len(apiKey) < 8 {
				fmt.Println("密钥格式可能不正确")
				fmt.Print("是否继续? (y/n): ")
				confirm, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
					continue
				}
			}
			break
		}

		// 获取 ARK_MODEL_ID
		var modelID string
		for {
			fmt.Print("请输入模型ID: ")
			input, _ := reader.ReadString('\n')
			modelID = strings.TrimSpace(input)

			if modelID == "" {
				fmt.Println("模型ID不能为空!")
				continue
			}
			break
		}

		// 创建配置文件
		content := fmt.Sprintf("ARK_API_KEY:%s\nARK_MODEL_ID:%s\n", apiKey, modelID)

		err := os.WriteFile(envFile, []byte(content), 0644)
		if err != nil {
			fmt.Printf("创建配置文件失败: %v\n", err)
			return err
		}

		fmt.Printf("\n配置文件已生成: %s\n", envFile)
		fmt.Println("您可以随时编辑此文件修改配置")

	} else {
		fmt.Println("环境配置文件检测正常")
		fmt.Println("是否重写环境配置信息？(y/n)")
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(confirm)) != "n" {
			err := InitEnv(true)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func isEnvFileValid(filename string) bool {
	content, err := os.ReadFile(filename)
	if err != nil {
		return false
	}

	lines := strings.Split(string(content), "\n")
	hasAPIKey := false
	hasModelID := false

	for _, line := range lines {
		if strings.Contains(line, "ARK_API_KEY") && len(line) > len("ARK_API_KEY:")+5 {
			hasAPIKey = true
		}
		if strings.Contains(line, "ARK_MODEL_ID") && len(line) > len("ARK_MODEL_ID:")+1 {
			hasModelID = true
		}
	}

	return hasAPIKey && hasModelID
}
