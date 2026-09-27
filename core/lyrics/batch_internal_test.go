package lyrics

import (
	"testing"
	"time"
)

// 回归：Cancel 以前会立刻把 Running 置 false，而 worker 只在两首歌之间检查
// 取消标志，正在进行的那次 TranslateSong 可能还要跑好几秒。这期间界面显示
// "已停止"，用户就能再点一次开始 —— 旧 worker 继续往新一轮的计数器里写，
// 并且在退出时把**新一轮**标记成已完成。
func TestBatchRetranslate_CancelDoesNotAllowOverlappingRuns(t *testing.T) {
	svc := NewTranslationService(nil)

	// 手工布置一个运行中的批次：worker 由测试直接驱动，不依赖真实 provider
	svc.batchMu.Lock()
	svc.batchGen++
	gen := svc.batchGen
	svc.batchCancel = make(chan struct{})
	svc.batchStatus = BatchRetranslateStatus{Running: true, Total: 3, StartedAt: time.Now()}
	svc.batchMu.Unlock()

	// 复刻 worker 的写入口径
	mutate := func(fn func(st *BatchRetranslateStatus)) {
		svc.batchMu.Lock()
		defer svc.batchMu.Unlock()
		if svc.batchGen != gen {
			return
		}
		fn(&svc.batchStatus)
	}

	svc.CancelBatchRetranslate()

	st := svc.GetBatchRetranslateStatus()
	if !st.Running {
		t.Fatal("cancel must not report the batch as finished while the worker is still draining")
	}
	if !st.Canceling {
		t.Fatal("expected Canceling to be reported so the UI can show a stopping state")
	}

	// 用户此时再启动一轮：新一轮拿到新的代次
	svc.batchMu.Lock()
	svc.batchGen++
	newGen := svc.batchGen
	svc.batchCancel = make(chan struct{})
	svc.batchStatus = BatchRetranslateStatus{Running: true, Total: 1, StartedAt: time.Now()}
	svc.batchMu.Unlock()

	// 旧 worker 收尾：既不能清掉新一轮的 Running，也不能往计数器里写
	mutate(func(st *BatchRetranslateStatus) {
		st.Running = false
		st.Canceling = false
		st.Current = ""
	})
	mutate(func(st *BatchRetranslateStatus) { st.Processed++ })

	st = svc.GetBatchRetranslateStatus()
	if !st.Running {
		t.Fatal("a stale worker must not mark the newer run as finished")
	}
	if st.Processed != 0 {
		t.Fatalf("a stale worker wrote %d into the newer run's counters", st.Processed)
	}
	if newGen == gen {
		t.Fatal("expected the new run to get a distinct generation")
	}
}
