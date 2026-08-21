package store

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"example.com/forma-orders-target/internal/domain"
)

const pageSize = 20

type replay struct {
	Kind string
	ID   string
}

type Store struct {
	mu sync.RWMutex

	nextID       int
	customers    map[string]domain.Customer
	products     map[string]domain.Product
	stockItems   map[string]domain.StockItem
	plans        map[string]domain.ReservationPlan
	orders       map[string]domain.Order
	orderLines   map[string]domain.OrderLine
	reservations map[string]domain.StockReservation
	orderNumbers map[string]string
	replays      map[string]replay

	// afterReservationSnapshotForTest lets a repository test alter the backing
	// value source after the action has captured its pre-state. Production
	// construction leaves it nil.
	afterReservationSnapshotForTest func()
}

func New() *Store {
	return &Store{
		customers: map[string]domain.Customer{}, products: map[string]domain.Product{},
		stockItems: map[string]domain.StockItem{}, plans: map[string]domain.ReservationPlan{}, orders: map[string]domain.Order{},
		orderLines: map[string]domain.OrderLine{}, reservations: map[string]domain.StockReservation{}, orderNumbers: map[string]string{},
		replays: map[string]replay{},
	}
}

func (store *Store) next(prefix string) string {
	store.nextID++
	return fmt.Sprintf("%s-%d", prefix, store.nextID)
}

func (store *Store) PutCustomer(customer domain.Customer) domain.Customer {
	store.mu.Lock()
	defer store.mu.Unlock()
	if customer.ID == "" {
		customer.ID = store.next("customer")
	}
	store.customers[customer.ID] = customer
	return customer
}

func (store *Store) PutProduct(product domain.Product) domain.Product {
	store.mu.Lock()
	defer store.mu.Unlock()
	if product.ID == "" {
		product.ID = store.next("product")
	}
	store.products[product.ID] = product
	return product
}

func (store *Store) PutStockItem(item domain.StockItem) (domain.StockItem, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := domain.ValidateStock(item); err != nil {
		return domain.StockItem{}, err
	}
	if item.ProductID != "" {
		if _, ok := store.products[item.ProductID]; !ok {
			return domain.StockItem{}, domain.ErrInvalid
		}
	}
	if item.ID == "" {
		item.ID = store.next("stock")
	}
	item.Version++
	store.stockItems[item.ID] = item
	return item, nil
}

func (store *Store) PutOrderLine(line domain.OrderLine) (domain.OrderLine, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if line.Quantity < 0 || store.orders[line.OrderID].ID == "" || store.products[line.ProductID].ID == "" {
		return domain.OrderLine{}, domain.ErrInvalid
	}
	if line.ID == "" {
		line.ID = store.next("line")
	}
	store.orderLines[line.ID] = line
	order := store.orders[line.OrderID]
	order.LineIDs = append(order.LineIDs, line.ID)
	store.orders[order.ID] = order
	return line, nil
}

type OrderInput struct {
	Number     string
	CustomerID string
}

func (store *Store) CreateOrder(input OrderInput, token string) (domain.Order, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if replayed, ok := store.replay("create-order", token); ok {
		return cloneOrder(store.orders[replayed.ID]), nil
	}
	order := domain.Order{Number: strings.TrimSpace(input.Number), CustomerID: input.CustomerID, Status: domain.OrderDraft}
	if err := domain.ValidateOrder(order); err != nil {
		return domain.Order{}, err
	}
	if input.CustomerID != "" && store.customers[input.CustomerID].ID == "" {
		return domain.Order{}, domain.ErrInvalid
	}
	if _, exists := store.orderNumbers[order.Number]; exists {
		return domain.Order{}, domain.ErrConflict
	}
	order.ID = store.next("order")
	order.Version = 1
	store.orders[order.ID] = order
	store.orderNumbers[order.Number] = order.ID
	store.remember("create-order", token, order.ID)
	return cloneOrder(order), nil
}

func (store *Store) UpdateOrder(id string, input OrderInput, token string) (domain.Order, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if replayed, ok := store.replay("update-order:"+id, token); ok {
		return cloneOrder(store.orders[replayed.ID]), nil
	}
	order, ok := store.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	want := order
	want.Number = strings.TrimSpace(input.Number)
	want.CustomerID = input.CustomerID
	if err := domain.ValidateOrder(want); err != nil {
		return domain.Order{}, err
	}
	if input.CustomerID != "" && store.customers[input.CustomerID].ID == "" {
		return domain.Order{}, domain.ErrInvalid
	}
	if owner, exists := store.orderNumbers[want.Number]; exists && owner != id {
		return domain.Order{}, domain.ErrConflict
	}
	delete(store.orderNumbers, order.Number)
	store.orderNumbers[want.Number] = id
	want.Version++
	store.orders[id] = want
	store.remember("update-order:"+id, token, id)
	return cloneOrder(want), nil
}

func (store *Store) DeleteOrder(id string, principal domain.Principal) error {
	if !domain.Allowed(principal, domain.OrderDelete) {
		return domain.ErrDenied
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	order, ok := store.orders[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(store.orders, id)
	delete(store.orderNumbers, order.Number)
	for _, lineID := range order.LineIDs {
		delete(store.orderLines, lineID)
	}
	return nil
}

func (store *Store) TransitionOrder(id string, action domain.Action, principal domain.Principal, confirmed bool) (domain.Order, error) {
	capability := map[domain.Action]domain.Capability{
		domain.ActionSubmit: domain.OrderSubmit, domain.ActionApprove: domain.OrderApprove,
		domain.ActionReject: domain.OrderReject, domain.ActionShip: domain.OrderShip,
	}[action]
	if !domain.Allowed(principal, capability) {
		return domain.Order{}, domain.ErrDenied
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	order, ok := store.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	status, err := domain.Transition(order.Status, action, confirmed)
	if err != nil {
		return domain.Order{}, err
	}
	order.Status = status
	order.Version++
	store.orders[id] = order
	return cloneOrder(order), nil
}

type StockInput struct {
	ProductID string
	Location  string
	OnHand    int
	Reserved  int
}

func (store *Store) UpdateStockItem(id string, input StockInput, token string) (domain.StockItem, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if replayed, ok := store.replay("update-stock:"+id, token); ok {
		return store.stockItems[replayed.ID], nil
	}
	current, ok := store.stockItems[id]
	if !ok {
		return domain.StockItem{}, domain.ErrNotFound
	}
	want := current
	want.ProductID = input.ProductID
	want.Location = strings.TrimSpace(input.Location)
	want.OnHand = input.OnHand
	want.Reserved = input.Reserved
	if err := domain.ValidateStock(want); err != nil {
		return domain.StockItem{}, err
	}
	if want.ProductID != "" && store.products[want.ProductID].ID == "" {
		return domain.StockItem{}, domain.ErrInvalid
	}
	want.Version++
	store.stockItems[id] = want
	store.remember("update-stock:"+id, token, id)
	return want, nil
}

// ReserveStock is a second authoritative mutation boundary over the same
// invariant fields. The mutex encloses both the post-state calculation and
// commit, so concurrent reservations cannot each validate a stale snapshot.
func (store *Store) ReserveStock(id string, quantity int) (domain.StockItem, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if quantity < 0 {
		return domain.StockItem{}, domain.ErrInvalid
	}
	item, ok := store.stockItems[id]
	if !ok {
		return domain.StockItem{}, domain.ErrNotFound
	}
	want := item
	want.Reserved += quantity
	if err := domain.ValidateStock(want); err != nil {
		return domain.StockItem{}, err
	}
	want.Version++
	store.stockItems[id] = want
	return want, nil
}

func (store *Store) PutReservationPlan(plan domain.ReservationPlan) (domain.ReservationPlan, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if strings.TrimSpace(plan.Code) == "" || plan.ApprovedReserved < 0 {
		return domain.ReservationPlan{}, domain.ErrInvalid
	}
	if plan.ID == "" {
		plan.ID = store.next("plan")
	}
	store.plans[plan.ID] = plan
	return plan, nil
}

func (store *Store) PutStockReservation(reservation domain.StockReservation) (domain.StockReservation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if strings.TrimSpace(reservation.Code) == "" || reservation.RequestedReserved < 0 ||
		store.stockItems[reservation.StockID].ID == "" || store.plans[reservation.PlanID].ID == "" {
		return domain.StockReservation{}, domain.ErrInvalid
	}
	if reservation.ID == "" {
		reservation.ID = store.next("reservation")
	}
	if reservation.Status == "" {
		reservation.Status = domain.ReservationPending
	}
	reservation.Version++
	store.reservations[reservation.ID] = reservation
	return reservation, nil
}

// CommitStockReservation implements one action-owned atomic boundary. The
// source state, relation target, distinct relation value, candidate StockItem
// invariant, and both commits are evaluated while holding the same lock.
func (store *Store) CommitStockReservation(id string, principal domain.Principal, confirmed bool) (domain.StockReservation, domain.StockItem, error) {
	if !domain.Allowed(principal, domain.ReservationCommit) {
		return domain.StockReservation{}, domain.StockItem{}, domain.ErrDenied
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	reservation, ok := store.reservations[id]
	if !ok {
		return domain.StockReservation{}, domain.StockItem{}, domain.ErrNotFound
	}
	if !confirmed {
		return domain.StockReservation{}, domain.StockItem{}, domain.ErrConfirmationNeeded
	}
	if reservation.Status != domain.ReservationPending {
		return domain.StockReservation{}, domain.StockItem{}, domain.ErrInvalidTransition
	}
	stock, ok := store.stockItems[reservation.StockID]
	if !ok {
		return domain.StockReservation{}, domain.StockItem{}, domain.ErrTargetUnavailable
	}
	plan, ok := store.plans[reservation.PlanID]
	if !ok {
		return domain.StockReservation{}, domain.StockItem{}, domain.ErrValueUnavailable
	}
	if store.afterReservationSnapshotForTest != nil {
		store.afterReservationSnapshotForTest()
	}
	wantStock := stock
	wantStock.Reserved = plan.ApprovedReserved
	if err := domain.ValidateStock(wantStock); err != nil {
		return domain.StockReservation{}, domain.StockItem{}, err
	}
	wantReservation := reservation
	wantReservation.Status = domain.ReservationCommitted
	wantReservation.Version++
	wantStock.Version++
	store.reservations[id] = wantReservation
	store.stockItems[stock.ID] = wantStock
	return wantReservation, wantStock, nil
}

func (store *Store) RemoveStockItem(id string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.stockItems, id)
}

func (store *Store) RemoveReservationPlan(id string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.plans, id)
}

func (store *Store) replay(kind, token string) (replay, bool) {
	if token == "" {
		return replay{}, false
	}
	item, ok := store.replays[kind+"\x00"+token]
	return item, ok
}

func (store *Store) remember(kind, token, id string) {
	if token != "" {
		store.replays[kind+"\x00"+token] = replay{Kind: kind, ID: id}
	}
}

func (store *Store) Customer(id string) (domain.Customer, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.customers[id]
	if !ok {
		return domain.Customer{}, domain.ErrNotFound
	}
	return item, nil
}

func (store *Store) Product(id string) (domain.Product, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.products[id]
	if !ok {
		return domain.Product{}, domain.ErrNotFound
	}
	return item, nil
}

func (store *Store) Order(id string) (domain.Order, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	return cloneOrder(item), nil
}

func (store *Store) StockItem(id string) (domain.StockItem, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.stockItems[id]
	if !ok {
		return domain.StockItem{}, domain.ErrNotFound
	}
	return item, nil
}

func (store *Store) StockReservation(id string) (domain.StockReservation, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.reservations[id]
	if !ok {
		return domain.StockReservation{}, domain.ErrNotFound
	}
	return item, nil
}

func (store *Store) ReservationPlan(id string) (domain.ReservationPlan, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.plans[id]
	if !ok {
		return domain.ReservationPlan{}, domain.ErrNotFound
	}
	return item, nil
}

func (store *Store) StockReservations() ([]domain.StockReservation, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	items := make([]domain.StockReservation, 0, len(store.reservations))
	for _, item := range store.reservations {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Code != items[j].Code {
			return items[i].Code < items[j].Code
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (store *Store) OrderLine(id string) (domain.OrderLine, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, ok := store.orderLines[id]
	if !ok {
		return domain.OrderLine{}, domain.ErrNotFound
	}
	return item, nil
}

type Page[T any] struct {
	Items   []T
	HasMore bool
}

type OrderQuery struct {
	Search     string
	Status     domain.OrderStatus
	CustomerID string
	Page       int
}

func (store *Store) Orders(query OrderQuery) (Page[domain.Order], error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	items := make([]domain.Order, 0, len(store.orders))
	for _, order := range store.orders {
		if query.Search != "" && !strings.Contains(strings.ToLower(order.Number), strings.ToLower(query.Search)) {
			continue
		}
		if query.Status != "" && order.Status != query.Status {
			continue
		}
		if query.CustomerID != "" && order.CustomerID != query.CustomerID {
			continue
		}
		items = append(items, cloneOrder(order))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Number != items[j].Number {
			return items[i].Number < items[j].Number
		}
		return items[i].ID < items[j].ID
	})
	return bounded(items, query.Page), nil
}

type StockQuery struct {
	Search    string
	ProductID string
	Page      int
}

func (store *Store) StockItems(query StockQuery) (Page[domain.StockItem], error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	items := make([]domain.StockItem, 0, len(store.stockItems))
	for _, item := range store.stockItems {
		if query.Search != "" && !strings.Contains(strings.ToLower(item.Location), strings.ToLower(query.Search)) {
			continue
		}
		if query.ProductID != "" && item.ProductID != query.ProductID {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Location != items[j].Location {
			return items[i].Location < items[j].Location
		}
		return items[i].ID < items[j].ID
	})
	return bounded(items, query.Page), nil
}

func bounded[T any](items []T, page int) Page[T] {
	if page < 1 {
		page = 1
	}
	lastPage := (len(items) + pageSize - 1) / pageSize
	if len(items) == 0 || page > lastPage {
		return Page[T]{Items: []T{}}
	}
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(items))
	return Page[T]{Items: items[start:end], HasMore: end < len(items)}
}

func cloneOrder(order domain.Order) domain.Order {
	order.LineIDs = append([]string(nil), order.LineIDs...)
	return order
}
