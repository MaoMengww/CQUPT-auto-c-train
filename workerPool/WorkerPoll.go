package workerPool

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

type TaskFunc func()

type Task struct {
	ID   int
	Func TaskFunc
}

type TaskResult struct {
	ID     int
	Result interface{}
	Err    error
}

type WorkerPool struct {
	taskChan    chan Task
	resultChan  chan TaskResult
	stopChan    chan struct{}
	wg          sync.WaitGroup
	SubmitSum   int64
	CompleteSum int64
}

func NewWorkerPool(workerCount, queueSize int) *WorkerPool {
	if workerCount < 1 {
		workerCount = runtime.NumCPU() * 2
	}
	if queueSize < workerCount {
		queueSize = workerCount * 100
	}

	//封装
	pool := &WorkerPool{
		taskChan:   make(chan Task, queueSize),
		resultChan: make(chan TaskResult, queueSize),
		stopChan:   make(chan struct{}),
	}

	//分配
	for i := 0; i < workerCount; i++ {
		pool.wg.Add(1)
		go pool.Consume()
	}

	return pool
}

func (p *WorkerPool) Produce(taskFunc TaskFunc) error {
	//封装
	task := Task{
		ID:   int(atomic.AddInt64(&p.SubmitSum, 1)), // 自增
		Func: taskFunc,
	}

	select {
	case p.taskChan <- task:
		return nil
	case <-p.stopChan:
		return errors.New("pool stopped")
	}
}

func (p *WorkerPool) Consume() {
	defer p.wg.Done()

	//处理任务
	for {
		select {
		case task, ok := <-p.taskChan:
			if !ok {
				return
			}

			//执行任务
			task.Func()

			//传入
			p.resultChan <- TaskResult{
				ID: task.ID,
			}

			atomic.AddInt64(&p.CompleteSum, 1)

		case <-p.stopChan:
			return
		}
	}
}

func (p *WorkerPool) GetResults() <-chan TaskResult {
	return p.resultChan
}

func (p *WorkerPool) Close() {
	close(p.taskChan)
	p.wg.Wait()
	close(p.resultChan)
	close(p.stopChan)
}

func (p *WorkerPool) GetInfo() (int64, int64) {
	return p.SubmitSum, p.CompleteSum
}
