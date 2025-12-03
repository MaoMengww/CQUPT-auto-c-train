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
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

var (
	mu      sync.Mutex
	scanner = bufio.NewScanner(os.Stdin)
	score   int
	failed  int
	wg      sync.WaitGroup
)

func main() {
	err := ai.InitAI()
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
	fmt.Println("输入并发量(同一时间最大刷题数字,不输入则默认为12)")
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
	fmt.Println("最终得分为", score, "分,得0分得题目数量为", failed, "个")
}

// StartWork 封装即将执行的任务
func StartWork(choi, username, password string, i int, score *int) workerPool.TaskFunc {
	return func() {
		GetScore(choi, username, password, i, score, &failed)
	}
}

func GetScore(choi, username, password string, i int, score *int, failed *int) {
	taskID := i + 1
	fmt.Printf("  --任务%v：开始执行 \n", taskID)
	var scoreStr string
	//创建chrome实例
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.ExecPath(`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`),
	)
	alloctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloctx)
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
		log.Printf("任务%v：登录失败,error: %v", taskID, err)
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
		wg.Done()
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
			wg.Done()
			return
		}
		fmt.Printf("  --任务%v：解题中...\n", taskID)
		answer, err := ai.Answer(question)
		if err != nil {
			log.Printf("任务%v：答案生成失败:err: %v\n", taskID, err)
			wg.Done()
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
			wg.Done()
			return
		}

		//计算单个任务得分
		fmt.Printf("  --任务%v：已解答,得分为%v分\n", taskID, scoreStr)
		scoreInt, _ := strconv.Atoi(scoreStr)
		mu.Lock()
		*score += scoreInt
		mu.Unlock()
		if scoreInt == 0 {
			mu.Lock()
			*failed += 1
			mu.Unlock()
		}

	} else {
		//出错
		log.Printf("任务%v：检测过程发生未知错误: %v\n", taskID, err)
		wg.Done()
		return
	}
	fmt.Println("已完成任务 ", taskID)
	wg.Done()
}
