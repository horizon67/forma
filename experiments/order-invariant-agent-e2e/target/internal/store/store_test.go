package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"example.com/forma-orders-target/internal/domain"
)

type fixture struct {
	store    *Store
	customer domain.Customer
	product  domain.Product
	stock    domain.StockItem
	order    domain.Order
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	repository := New()
	customer := repository.PutCustomer(domain.Customer{Name: "Aster Labs", Email: "orders@aster.example"})
	product := repository.PutProduct(domain.Product{SKU: "WIDGET-1", Name: "Widget", Price: "12.50"})
	stock, err := repository.PutStockItem(domain.StockItem{ProductID: product.ID, Location: "Tokyo", OnHand: 10, Reserved: 2})
	if err != nil {
		t.Fatal(err)
	}
	order, err := repository.CreateOrder(OrderInput{Number: "ORD-100", CustomerID: customer.ID}, "seed-order")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{store: repository, customer: customer, product: product, stock: stock, order: order}
}

func TestRelationsResolveToEntitiesAndLabels(t *testing.T) {
	item := newFixture(t)
	line, err := item.store.PutOrderLine(domain.OrderLine{OrderID: item.order.ID, ProductID: item.product.ID, Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}
	order, _ := item.store.Order(item.order.ID)
	customer, _ := item.store.Customer(order.CustomerID)
	stockProduct, _ := item.store.Product(item.stock.ProductID)
	storedLine, _ := item.store.OrderLine(line.ID)
	lineOrder, _ := item.store.Order(storedLine.OrderID)
	lineProduct, _ := item.store.Product(storedLine.ProductID)
	if customer.Label() != "Aster Labs" || stockProduct.Label() != "Widget" ||
		lineOrder.Label() != "ORD-100" || lineProduct.Label() != "Widget" || len(order.LineIDs) != 1 {
		t.Fatalf("relations did not resolve: customer=%#v stockProduct=%#v line=%#v order=%#v", customer, stockProduct, storedLine, order)
	}
}

func TestOrderMutationsAreAcceptedAndAppliedAtMostOnce(t *testing.T) {
	item := newFixture(t)
	created, err := item.store.CreateOrder(OrderInput{Number: "ORD-200", CustomerID: item.customer.ID}, "create-once")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := item.store.CreateOrder(OrderInput{Number: "IGNORED", CustomerID: item.customer.ID}, "create-once")
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("create replay = %#v, %v", replayed, err)
	}
	updated, err := item.store.UpdateOrder(created.ID, OrderInput{Number: "ORD-201", CustomerID: item.customer.ID}, "edit-once")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err = item.store.UpdateOrder(created.ID, OrderInput{Number: "IGNORED", CustomerID: item.customer.ID}, "edit-once")
	if err != nil || replayed.Number != "ORD-201" || replayed.Version != updated.Version {
		t.Fatalf("edit replay = %#v, %v; first=%#v", replayed, err, updated)
	}
	page, _ := item.store.Orders(OrderQuery{})
	if len(page.Items) != 2 {
		t.Fatalf("orders = %d, want two logical mutations", len(page.Items))
	}
}

func TestOrderValidationRejectsRequiredAndUnique(t *testing.T) {
	item := newFixture(t)
	if _, err := item.store.CreateOrder(OrderInput{CustomerID: item.customer.ID}, ""); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("missing number error = %v", err)
	}
	if _, err := item.store.CreateOrder(OrderInput{Number: item.order.Number, CustomerID: item.customer.ID}, ""); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate number error = %v", err)
	}
	other, err := item.store.CreateOrder(OrderInput{Number: "ORD-OTHER", CustomerID: item.customer.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := item.store.UpdateOrder(other.ID, OrderInput{Number: item.order.Number, CustomerID: item.customer.ID}, ""); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate update error = %v", err)
	}
	stored, _ := item.store.Order(other.ID)
	if stored.Number != "ORD-OTHER" {
		t.Fatalf("rejected update changed stored order: %#v", stored)
	}
}

func TestOrderQuerySearchFiltersSortAndPageBoundary(t *testing.T) {
	item := newFixture(t)
	otherCustomer := item.store.PutCustomer(domain.Customer{Name: "Beacon", Email: "orders@beacon.example"})
	for index := 0; index < 22; index++ {
		order, err := item.store.CreateOrder(OrderInput{
			Number: fmt.Sprintf("ORD-%03d", 300+index), CustomerID: otherCustomer.ID,
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if _, err := item.store.TransitionOrder(order.ID, domain.ActionSubmit, domain.NewPrincipal(domain.RoleStaff), false); err != nil {
				t.Fatal(err)
			}
		}
	}
	page, err := item.store.Orders(OrderQuery{Search: "301", CustomerID: otherCustomer.ID})
	if err != nil || len(page.Items) != 1 || page.Items[0].Number != "ORD-301" {
		t.Fatalf("search/filter page = %#v, %v", page, err)
	}
	submitted, _ := item.store.Orders(OrderQuery{Status: domain.OrderSubmitted})
	if len(submitted.Items) != 1 || submitted.Items[0].Number != "ORD-300" {
		t.Fatalf("status filter = %#v", submitted)
	}
	all, _ := item.store.Orders(OrderQuery{})
	if len(all.Items) != 20 || !all.HasMore {
		t.Fatalf("bounded page = %d, more=%t", len(all.Items), all.HasMore)
	}
	remainder, _ := item.store.Orders(OrderQuery{Page: 2})
	if len(remainder.Items) != 3 || remainder.HasMore {
		t.Fatalf("remainder page = %d, more=%t", len(remainder.Items), remainder.HasMore)
	}
	for index := 1; index < len(all.Items); index++ {
		if all.Items[index-1].Number > all.Items[index].Number {
			t.Fatalf("orders are not sorted: %s then %s", all.Items[index-1].Number, all.Items[index].Number)
		}
	}
}

func TestOrderTransitionsHonorPreconditionsConfirmationAndRoles(t *testing.T) {
	item := newFixture(t)
	staff := domain.NewPrincipal(domain.RoleStaff)
	admin := domain.NewPrincipal(domain.RoleAdmin)
	order, err := item.store.TransitionOrder(item.order.ID, domain.ActionSubmit, staff, false)
	if err != nil || order.Status != domain.OrderSubmitted {
		t.Fatalf("submit = %#v, %v", order, err)
	}
	if _, err := item.store.TransitionOrder(item.order.ID, domain.ActionSubmit, staff, false); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("repeated submit error = %v", err)
	}
	if _, err := item.store.TransitionOrder(item.order.ID, domain.ActionApprove, staff, true); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("staff approve error = %v", err)
	}
	if _, err := item.store.TransitionOrder(item.order.ID, domain.ActionApprove, admin, false); !errors.Is(err, domain.ErrConfirmationNeeded) {
		t.Fatalf("unconfirmed approve error = %v", err)
	}
	order, err = item.store.TransitionOrder(item.order.ID, domain.ActionApprove, admin, true)
	if err != nil || order.Status != domain.OrderApproved {
		t.Fatalf("approve = %#v, %v", order, err)
	}
	order, err = item.store.TransitionOrder(item.order.ID, domain.ActionShip, staff, false)
	if err != nil || order.Status != domain.OrderShipped {
		t.Fatalf("ship = %#v, %v", order, err)
	}
}

func TestStockMutationIsAcceptedAndAppliedAtMostOnce(t *testing.T) {
	item := newFixture(t)
	updated, err := item.store.UpdateStockItem(item.stock.ID, StockInput{
		ProductID: item.product.ID, Location: "Osaka", OnHand: 12, Reserved: 4,
	}, "stock-once")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := item.store.UpdateStockItem(item.stock.ID, StockInput{
		ProductID: item.product.ID, Location: "Ignored", OnHand: 1, Reserved: 0,
	}, "stock-once")
	if err != nil || replayed != updated {
		t.Fatalf("stock replay = %#v, %v; first=%#v", replayed, err, updated)
	}
}

func TestStockValidationAndInvariantRejectAtomically(t *testing.T) {
	item := newFixture(t)
	cases := []StockInput{
		{ProductID: item.product.ID, Location: "", OnHand: 10, Reserved: 2},
		{ProductID: item.product.ID, Location: "Tokyo", OnHand: -1, Reserved: 0},
		{ProductID: item.product.ID, Location: "Tokyo", OnHand: 10, Reserved: -1},
		{ProductID: item.product.ID, Location: "Tokyo", OnHand: 1, Reserved: 5},
	}
	for _, input := range cases {
		if _, err := item.store.UpdateStockItem(item.stock.ID, input, ""); err == nil {
			t.Fatalf("invalid stock update succeeded: %#v", input)
		}
		stored, _ := item.store.StockItem(item.stock.ID)
		if stored != item.stock {
			t.Fatalf("rejected update left a partial change: got %#v, want %#v", stored, item.stock)
		}
	}
}

func TestStockInvariantAcceptsValidAndRejectsInvalidPostStates(t *testing.T) {
	item := newFixture(t)
	valid, err := item.store.UpdateStockItem(item.stock.ID, StockInput{
		ProductID: item.product.ID, Location: item.stock.Location, OnHand: 8, Reserved: 8,
	}, "")
	if err != nil || valid.Reserved > valid.OnHand {
		t.Fatalf("valid post-state = %#v, %v", valid, err)
	}
	before := valid
	if _, err := item.store.UpdateStockItem(item.stock.ID, StockInput{
		ProductID: item.product.ID, Location: item.stock.Location, OnHand: 7, Reserved: 8,
	}, ""); !errors.Is(err, domain.ErrInvariant) {
		t.Fatalf("invalid post-state error = %v", err)
	}
	after, _ := item.store.StockItem(item.stock.ID)
	if after != before {
		t.Fatalf("invariant rejection changed state: before=%#v after=%#v", before, after)
	}
}

func TestConcurrentReservationsPreserveStockInvariant(t *testing.T) {
	item := newFixture(t)
	if _, err := item.store.UpdateStockItem(item.stock.ID, StockInput{
		ProductID: item.product.ID, Location: item.stock.Location, OnHand: 10, Reserved: 0,
	}, ""); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := item.store.ReserveStock(item.stock.ID, 6)
			errorsSeen <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	succeeded, rejected := 0, 0
	for err := range errorsSeen {
		if err == nil {
			succeeded++
		} else if errors.Is(err, domain.ErrInvariant) {
			rejected++
		} else {
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	stored, _ := item.store.StockItem(item.stock.ID)
	if succeeded != 1 || rejected != 1 || stored.Reserved != 6 || stored.Reserved > stored.OnHand {
		t.Fatalf("concurrent result: success=%d rejected=%d stock=%#v", succeeded, rejected, stored)
	}
}

func TestStockQuerySearchFilterSortAndPageBoundary(t *testing.T) {
	item := newFixture(t)
	other := item.store.PutProduct(domain.Product{SKU: "OTHER-1", Name: "Other", Price: "2.00"})
	for index := 0; index < 21; index++ {
		productID := item.product.ID
		if index == 0 {
			productID = other.ID
		}
		if _, err := item.store.PutStockItem(domain.StockItem{
			ProductID: productID, Location: fmt.Sprintf("Warehouse-%02d", index), OnHand: 5,
		}); err != nil {
			t.Fatal(err)
		}
	}
	page, _ := item.store.StockItems(StockQuery{Search: "Warehouse-01", ProductID: item.product.ID})
	if len(page.Items) != 1 || page.Items[0].Location != "Warehouse-01" {
		t.Fatalf("stock search/filter = %#v", page)
	}
	all, _ := item.store.StockItems(StockQuery{})
	if len(all.Items) != 20 || !all.HasMore {
		t.Fatalf("stock page = %d, more=%t", len(all.Items), all.HasMore)
	}
	remainder, _ := item.store.StockItems(StockQuery{Page: 2})
	if len(remainder.Items) != 2 || remainder.HasMore {
		t.Fatalf("stock remainder page = %d, more=%t", len(remainder.Items), remainder.HasMore)
	}
	for index := 1; index < len(all.Items); index++ {
		if all.Items[index-1].Location > all.Items[index].Location {
			t.Fatalf("stock is not sorted: %s then %s", all.Items[index-1].Location, all.Items[index].Location)
		}
	}
}
