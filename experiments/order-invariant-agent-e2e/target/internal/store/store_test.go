package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"example.com/forma-orders-target/internal/domain"
)

type fixture struct {
	store       *Store
	customer    domain.Customer
	product     domain.Product
	stock       domain.StockItem
	reservation domain.StockReservation
	order       domain.Order
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
	reservation, err := repository.PutStockReservation(domain.StockReservation{Code: "RES-100", StockID: stock.ID, ReservedAfter: 6})
	if err != nil {
		t.Fatal(err)
	}
	order, err := repository.CreateOrder(OrderInput{Number: "ORD-100", CustomerID: customer.ID}, "seed-order")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{store: repository, customer: customer, product: product, stock: stock, reservation: reservation, order: order}
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
	reservation, _ := item.store.StockReservation(item.reservation.ID)
	reservationStock, _ := item.store.StockItem(reservation.StockID)
	storedLine, _ := item.store.OrderLine(line.ID)
	lineOrder, _ := item.store.Order(storedLine.OrderID)
	lineProduct, _ := item.store.Product(storedLine.ProductID)
	if customer.Label() != "Aster Labs" || stockProduct.Label() != "Widget" || reservationStock.Label() != "Tokyo" ||
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

func TestOrderSubmitTransitionFactsAtStoreBoundary(t *testing.T) {
	testOrderTransitionFactsAtStoreBoundary(t, domain.ActionSubmit, domain.OrderDraft, domain.OrderSubmitted, domain.NewPrincipal(domain.RoleStaff), false)
}

func TestOrderApproveTransitionFactsAtStoreBoundary(t *testing.T) {
	testOrderTransitionFactsAtStoreBoundary(t, domain.ActionApprove, domain.OrderSubmitted, domain.OrderApproved, domain.NewPrincipal(domain.RoleAdmin), true)
}

func TestOrderRejectTransitionFactsAtStoreBoundary(t *testing.T) {
	testOrderTransitionFactsAtStoreBoundary(t, domain.ActionReject, domain.OrderSubmitted, domain.OrderRejected, domain.NewPrincipal(domain.RoleAdmin), false)
}

func TestOrderShipTransitionFactsAtStoreBoundary(t *testing.T) {
	testOrderTransitionFactsAtStoreBoundary(t, domain.ActionShip, domain.OrderApproved, domain.OrderShipped, domain.NewPrincipal(domain.RoleStaff), false)
}

func testOrderTransitionFactsAtStoreBoundary(t *testing.T, action domain.Action, source, destination domain.OrderStatus, principal domain.Principal, confirmed bool) {
	t.Helper()
	states := []domain.OrderStatus{
		domain.OrderDraft, domain.OrderSubmitted, domain.OrderApproved, domain.OrderRejected, domain.OrderShipped,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			item := newFixture(t)
			order := createStoredOrderAtStatus(t, item, "STORE-"+string(action)+"-"+string(state), state)
			beforeVersion := order.Version
			got, err := item.store.TransitionOrder(order.ID, action, principal, confirmed)
			if state == source {
				if err != nil || got.Status != destination || got.Version != beforeVersion+1 {
					t.Fatalf("accepted transition = %#v, %v", got, err)
				}
				return
			}
			if !errors.Is(err, domain.ErrInvalidTransition) {
				t.Fatalf("rejected transition error = %v", err)
			}
			stored, readErr := item.store.Order(order.ID)
			if readErr != nil || stored.Status != state || stored.Version != beforeVersion {
				t.Fatalf("rejected transition changed order = %#v, %v", stored, readErr)
			}
		})
	}
}

func TestStockReservationCommitIsAtomicAcrossReservationAndStock(t *testing.T) {
	staff := domain.NewPrincipal(domain.RoleStaff)

	t.Run("accepted", func(t *testing.T) {
		item := newFixture(t)
		reservation, stock, err := item.store.CommitStockReservation(item.reservation.ID, staff, true)
		if err != nil || reservation.Status != domain.ReservationCommitted || stock.Reserved != item.reservation.ReservedAfter {
			t.Fatalf("commit = %#v, %#v, %v", reservation, stock, err)
		}
	})

	t.Run("invariant rejection preserves both entities", func(t *testing.T) {
		item := newFixture(t)
		reservation, err := item.store.PutStockReservation(domain.StockReservation{
			Code: "RES-INVALID", StockID: item.stock.ID, ReservedAfter: item.stock.OnHand + 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := item.store.CommitStockReservation(reservation.ID, staff, true); !errors.Is(err, domain.ErrInvariant) {
			t.Fatalf("invariant error = %v", err)
		}
		storedReservation, _ := item.store.StockReservation(reservation.ID)
		storedStock, _ := item.store.StockItem(item.stock.ID)
		if storedReservation.Status != domain.ReservationPending || storedStock.Reserved != item.stock.Reserved {
			t.Fatalf("partial commit after invariant rejection: reservation=%#v stock=%#v", storedReservation, storedStock)
		}
	})

	t.Run("target unavailable preserves source", func(t *testing.T) {
		item := newFixture(t)
		item.store.RemoveStockItem(item.stock.ID)
		if _, _, err := item.store.CommitStockReservation(item.reservation.ID, staff, true); !errors.Is(err, domain.ErrTargetUnavailable) {
			t.Fatalf("target-unavailable error = %v", err)
		}
		stored, _ := item.store.StockReservation(item.reservation.ID)
		if stored.Status != domain.ReservationPending || stored.Version != item.reservation.Version {
			t.Fatalf("target-unavailable changed source = %#v", stored)
		}
	})

	t.Run("source rejection preserves target", func(t *testing.T) {
		item := newFixture(t)
		if _, _, err := item.store.CommitStockReservation(item.reservation.ID, staff, true); err != nil {
			t.Fatal(err)
		}
		before, _ := item.store.StockItem(item.stock.ID)
		if _, _, err := item.store.CommitStockReservation(item.reservation.ID, staff, true); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("source rejection error = %v", err)
		}
		after, _ := item.store.StockItem(item.stock.ID)
		if after != before {
			t.Fatalf("source rejection changed target: before=%#v after=%#v", before, after)
		}
	})
}

func TestStockReservationCommitAuthorizationOwnsCrossEntityWritePath(t *testing.T) {
	item := newFixture(t)
	if _, _, err := item.store.CommitStockReservation(item.reservation.ID, domain.NewPrincipal(domain.RoleAdmin), true); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("admin commit error = %v", err)
	}
	storedReservation, _ := item.store.StockReservation(item.reservation.ID)
	storedStock, _ := item.store.StockItem(item.stock.ID)
	if storedReservation.Status != domain.ReservationPending || storedStock.Reserved != item.stock.Reserved {
		t.Fatalf("denied cross-entity write changed state: reservation=%#v stock=%#v", storedReservation, storedStock)
	}
	if _, _, err := item.store.CommitStockReservation(item.reservation.ID, domain.NewPrincipal(domain.RoleStaff), true); err != nil {
		t.Fatalf("staff commit was denied by target entity's admin-only edit surface: %v", err)
	}
}

func TestConcurrentStockReservationCommitsCannotPartiallyViolateInvariant(t *testing.T) {
	item := newFixture(t)
	valid, err := item.store.PutStockReservation(domain.StockReservation{Code: "RES-CONCURRENT-VALID", StockID: item.stock.ID, ReservedAfter: 8})
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := item.store.PutStockReservation(domain.StockReservation{Code: "RES-CONCURRENT-INVALID", StockID: item.stock.ID, ReservedAfter: 11})
	if err != nil {
		t.Fatal(err)
	}
	staff := domain.NewPrincipal(domain.RoleStaff)
	start := make(chan struct{})
	errorsByID := make(chan struct {
		id  string
		err error
	}, 2)
	for _, reservation := range []domain.StockReservation{valid, invalid} {
		reservation := reservation
		go func() {
			<-start
			_, _, commitErr := item.store.CommitStockReservation(reservation.ID, staff, true)
			errorsByID <- struct {
				id  string
				err error
			}{reservation.ID, commitErr}
		}()
	}
	close(start)
	results := map[string]error{}
	for range 2 {
		result := <-errorsByID
		results[result.id] = result.err
	}
	if results[valid.ID] != nil || !errors.Is(results[invalid.ID], domain.ErrInvariant) {
		t.Fatalf("concurrent results = %#v", results)
	}
	storedValid, _ := item.store.StockReservation(valid.ID)
	storedInvalid, _ := item.store.StockReservation(invalid.ID)
	stock, _ := item.store.StockItem(item.stock.ID)
	if storedValid.Status != domain.ReservationCommitted || storedInvalid.Status != domain.ReservationPending || stock.Reserved != 8 {
		t.Fatalf("concurrent atomic state = valid %#v invalid %#v stock %#v", storedValid, storedInvalid, stock)
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

func createStoredOrderAtStatus(t *testing.T, item fixture, number string, status domain.OrderStatus) domain.Order {
	t.Helper()
	order, err := item.store.CreateOrder(OrderInput{Number: number, CustomerID: item.customer.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	staff := domain.NewPrincipal(domain.RoleStaff)
	admin := domain.NewPrincipal(domain.RoleAdmin)
	if status != domain.OrderDraft {
		order, err = item.store.TransitionOrder(order.ID, domain.ActionSubmit, staff, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	switch status {
	case domain.OrderDraft, domain.OrderSubmitted:
	case domain.OrderApproved, domain.OrderShipped:
		order, err = item.store.TransitionOrder(order.ID, domain.ActionApprove, admin, true)
		if err != nil {
			t.Fatal(err)
		}
		if status == domain.OrderShipped {
			order, err = item.store.TransitionOrder(order.ID, domain.ActionShip, staff, false)
			if err != nil {
				t.Fatal(err)
			}
		}
	case domain.OrderRejected:
		order, err = item.store.TransitionOrder(order.ID, domain.ActionReject, admin, false)
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported order status %s", status)
	}
	return order
}
