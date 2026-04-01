package batch

import (
	"TODOLIST_Tasks/app/internal/tasks/domain"
	"TODOLIST_Tasks/app/internal/tasks/service"
	logging "TODOLIST_Tasks/app/pkg/logging"
	"context"
	"strings"
	"time"
)

const (
	taskChannelSize = 10000
	taskMaxBatch    = 500
	taskFlushMs     = 10

	deleteChannelSize = 10000
	deleteMaxBatch    = 500
	deleteFlushMs     = 10

	goroutineTimeout      = 5 * time.Second
	redisConcurrencyLimit = 500
)

// DeleteItem — элемент очереди batch-удаления.
type DeleteItem struct {
	ID     string
	UserID string
}

// Processor — батч-обработчик задач с поддержкой graceful shutdown.
// Содержит бизнес-логику батчевой вставки/удаления, вынесенную из delivery слоя.
type Processor struct {
	cmd       service.TaskCommandService
	cache     service.TaskCacheService
	logger    *logging.Logger
	taskCh    chan domain.Task
	deleteCh  chan DeleteItem
	redisSema chan struct{}
}

// New создаёт Processor. Запуск воркеров — через Start(ctx).
func New(cmd service.TaskCommandService, cache service.TaskCacheService, logger *logging.Logger) *Processor {
	return &Processor{
		cmd:       cmd,
		cache:     cache,
		logger:    logger.GetLoggerWithField("component", "batch"),
		taskCh:    make(chan domain.Task, taskChannelSize),
		deleteCh:  make(chan DeleteItem, deleteChannelSize),
		redisSema: make(chan struct{}, redisConcurrencyLimit),
	}
}

// Start запускает batch-воркеры в фоне. При отмене ctx — воркеры сбрасывают остатки и завершаются.
func (p *Processor) Start(ctx context.Context) {
	go p.runCreateWorker(ctx)
	go p.runDeleteWorker(ctx)
}

// EnqueueCreate ставит задачу в очередь batch-вставки.
// Возвращает false если канал переполнен (нужен fallback на синхронную вставку).
func (p *Processor) EnqueueCreate(task domain.Task) bool {
	select {
	case p.taskCh <- task:
		return true
	default:
		return false
	}
}

// EnqueueDelete ставит задачу в очередь batch-удаления.
// Возвращает false если канал переполнен.
func (p *Processor) EnqueueDelete(id, userID string) bool {
	select {
	case p.deleteCh <- DeleteItem{ID: id, UserID: userID}:
		return true
	default:
		return false
	}
}

func (p *Processor) runCreateWorker(ctx context.Context) {
	ticker := time.NewTicker(taskFlushMs * time.Millisecond)
	defer ticker.Stop()
	batch := make([]domain.Task, 0, taskMaxBatch)

	for {
		select {
		case task := <-p.taskCh:
			batch = append(batch, task)
			if len(batch) >= taskMaxBatch {
				p.flushCreate(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				p.flushCreate(batch)
				batch = batch[:0]
			}
		case <-ctx.Done():
			// Сбрасываем накопленный batch
			if len(batch) > 0 {
				p.flushCreate(batch)
				batch = batch[:0]
			}
			// Дочитываем оставшиеся задачи из канала
			for {
				select {
				case task := <-p.taskCh:
					batch = append(batch, task)
					if len(batch) >= taskMaxBatch {
						p.flushCreate(batch)
						batch = batch[:0]
					}
				default:
					if len(batch) > 0 {
						p.flushCreate(batch)
					}
					return
				}
			}
		}
	}
}

// flushCreate сохраняет накопленные задачи.
// Задачи с TagName — через одиночный CreateTask (CTE резолв тега),
// остальные — одним batch INSERT.
func (p *Processor) flushCreate(tasks []domain.Task) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	simple := make([]domain.Task, 0, len(tasks))
	for _, t := range tasks {
		if strings.TrimSpace(t.TagName) != "" {
			if err := p.cmd.CreateTask(ctx, t); err != nil {
				p.logger.Errorf("flushCreate: tagged task %s: %v", t.ID, err)
			}
		} else {
			simple = append(simple, t)
		}
	}
	if len(simple) > 0 {
		if err := p.cmd.CreateTaskBatch(ctx, simple); err != nil {
			p.logger.Errorf("flushCreate: batch %d tasks: %v", len(simple), err)
		}
	}

	// Асинхронное кэширование с ограничением параллелизма
	for _, task := range tasks {
		t := task
		select {
		case p.redisSema <- struct{}{}:
			go func(t domain.Task) {
				defer func() { <-p.redisSema }()
				bgCtx, bCancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer bCancel()
				if err := p.cache.SetTask(bgCtx, t); err != nil {
					p.logger.Warnf("flushCreate: cache task %s: %v", t.ID, err)
				}
				_ = p.cache.InvalidateUserLists(bgCtx, t.UserID)
			}(t)
		default:
			p.logger.Warnf("redis semaphore full, skip cache for task %s", t.ID)
		}
	}
}

func (p *Processor) runDeleteWorker(ctx context.Context) {
	ticker := time.NewTicker(deleteFlushMs * time.Millisecond)
	defer ticker.Stop()
	batch := make([]DeleteItem, 0, deleteMaxBatch)

	for {
		select {
		case item := <-p.deleteCh:
			batch = append(batch, item)
			if len(batch) >= deleteMaxBatch {
				p.flushDelete(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				p.flushDelete(batch)
				batch = batch[:0]
			}
		case <-ctx.Done():
			if len(batch) > 0 {
				p.flushDelete(batch)
				batch = batch[:0]
			}
			for {
				select {
				case item := <-p.deleteCh:
					batch = append(batch, item)
					if len(batch) >= deleteMaxBatch {
						p.flushDelete(batch)
						batch = batch[:0]
					}
				default:
					if len(batch) > 0 {
						p.flushDelete(batch)
					}
					return
				}
			}
		}
	}
}

func (p *Processor) flushDelete(items []DeleteItem) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	if err := p.cmd.DeleteTaskBatch(ctx, ids); err != nil {
		p.logger.Errorf("flushDelete: %d tasks: %v", len(ids), err)
	}

	for _, item := range items {
		it := item
		select {
		case p.redisSema <- struct{}{}:
			go func(id, userID string) {
				defer func() { <-p.redisSema }()
				bgCtx, bCancel := context.WithTimeout(context.Background(), goroutineTimeout)
				defer bCancel()
				_ = p.cache.DeleteCachedTask(bgCtx, id)
				if userID != "" {
					_ = p.cache.InvalidateUserLists(bgCtx, userID)
				}
			}(it.ID, it.UserID)
		default:
			p.logger.Warnf("redis semaphore full, skip cache delete for task %s", it.ID)
		}
	}
}
