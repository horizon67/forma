package store

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"example.com/forma-orders-target/internal/domain"
)

type fixture struct {
	store       *Store
	customer    domain.Customer
	product     domain.Product
	stock       domain.StockItem
	plan        domain.ReservationPlan
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
	plan, err := repository.PutReservationPlan(domain.ReservationPlan{Code: "PLAN-100", ApprovedReserved: 6, RequestCeiling: 6})
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := repository.PutStockReservation(domain.StockReservation{
		Code: "RES-100", StockID: stock.ID, PlanID: plan.ID, RequestedReserved: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := repository.CreateOrder(OrderInput{Number: "ORD-100", CustomerID: customer.ID}, "seed-order")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{store: repository, customer: customer, product: product, stock: stock, plan: plan, reservation: reservation, order: order}
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
	reservationPlan, _ := item.store.ReservationPlan(reservation.PlanID)
	storedLine, _ := item.store.OrderLine(line.ID)
	lineOrder, _ := item.store.Order(storedLine.OrderID)
	lineProduct, _ := item.store.Product(storedLine.ProductID)
	if customer.Label() != "Aster Labs" || stockProduct.Label() != "Widget" || reservationStock.Label() != "Tokyo" || reservationPlan.Label() != "PLAN-100" ||
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
		wantReserved := item.stock.Reserved + item.plan.ApprovedReserved
		if err != nil || reservation.Status != domain.ReservationCommitted || stock.Reserved != wantReserved ||
			stock.Reserved == item.stock.Reserved || stock.Reserved == item.plan.ApprovedReserved || stock.Reserved == item.reservation.RequestedReserved {
			t.Fatalf("commit = %#v, %#v, %v", reservation, stock, err)
		}
	})

	t.Run("equality boundary", func(t *testing.T) {
		item := newFixture(t)
		plan := item.plan
		plan.RequestCeiling = item.stock.Reserved + item.reservation.RequestedReserved
		if _, err := item.store.PutReservationPlan(plan); err != nil {
			t.Fatal(err)
		}
		reservation, stock, err := item.store.CommitStockReservation(item.reservation.ID, staff, true)
		if err != nil || reservation.Status != domain.ReservationCommitted || stock.Reserved != 8 {
			t.Fatalf("equality-boundary commit = %#v, %#v, %v", reservation, stock, err)
		}
	})

	t.Run("precondition rejection preserves every subject", func(t *testing.T) {
		item := newFixture(t)
		plan := item.plan
		plan.RequestCeiling = 4
		if _, err := item.store.PutReservationPlan(plan); err != nil {
			t.Fatal(err)
		}
		if _, _, err := item.store.CommitStockReservation(item.reservation.ID, staff, true); !errors.Is(err, domain.ErrPrecondition) {
			t.Fatalf("precondition error = %v", err)
		}
		storedReservation, _ := item.store.StockReservation(item.reservation.ID)
		storedStock, _ := item.store.StockItem(item.stock.ID)
		storedPlan, _ := item.store.ReservationPlan(item.plan.ID)
		if storedReservation.Status != domain.ReservationPending || storedReservation.Version != item.reservation.Version ||
			storedStock != item.stock || storedPlan.RequestCeiling != 4 {
			t.Fatalf("precondition rejection changed subjects: reservation=%#v stock=%#v plan=%#v", storedReservation, storedStock, storedPlan)
		}
	})

	t.Run("accepted value is the captured pre-state", func(t *testing.T) {
		item := newFixture(t)
		item.store.afterReservationSnapshotForTest = func() {
			changed := item.store.plans[item.plan.ID]
			changed.ApprovedReserved = 9
			changed.RequestCeiling = 1
			item.store.plans[item.plan.ID] = changed
			changedReservation := item.store.reservations[item.reservation.ID]
			changedReservation.RequestedReserved = 100
			item.store.reservations[item.reservation.ID] = changedReservation
			changedStock := item.store.stockItems[item.stock.ID]
			changedStock.Reserved = 1
			item.store.stockItems[item.stock.ID] = changedStock
		}
		reservation, stock, err := item.store.CommitStockReservation(item.reservation.ID, staff, true)
		if err != nil || stock.Reserved != item.stock.Reserved+item.plan.ApprovedReserved ||
			reservation.RequestedReserved != item.reservation.RequestedReserved {
			t.Fatalf("commit reread an operand after its pre-state snapshot: reservation=%#v stock=%#v error=%v", reservation, stock, err)
		}
		changedPlan, _ := item.store.ReservationPlan(item.plan.ID)
		if changedPlan.ApprovedReserved != 9 || changedPlan.RequestCeiling != 1 {
			t.Fatalf("snapshot hook did not alter the backing value source: plan=%#v", changedPlan)
		}
	})

	t.Run("exact predicate overflow rejects as invalid", func(t *testing.T) {
		item := newFixture(t)
		maximum := int(^uint(0) >> 1)
		stock, err := item.store.PutStockItem(domain.StockItem{ProductID: item.product.ID, Location: "Predicate overflow", OnHand: maximum, Reserved: maximum})
		if err != nil {
			t.Fatal(err)
		}
		plan, err := item.store.PutReservationPlan(domain.ReservationPlan{Code: "PLAN-PREDICATE-OVERFLOW", ApprovedReserved: 0, RequestCeiling: maximum})
		if err != nil {
			t.Fatal(err)
		}
		reservation, err := item.store.PutStockReservation(domain.StockReservation{
			Code: "RES-PREDICATE-OVERFLOW", StockID: stock.ID, PlanID: plan.ID, RequestedReserved: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := item.store.CommitStockReservation(reservation.ID, staff, true); !errors.Is(err, domain.ErrPrecondition) {
			t.Fatalf("exact predicate error = %v", err)
		}
		storedReservation, _ := item.store.StockReservation(reservation.ID)
		storedStock, _ := item.store.StockItem(stock.ID)
		if storedReservation.Status != domain.ReservationPending || storedStock != stock {
			t.Fatalf("predicate overflow changed state: reservation=%#v stock=%#v", storedReservation, storedStock)
		}
	})

	t.Run("unrepresentable exact result preserves both entities", func(t *testing.T) {
		item := newFixture(t)
		maximum := int(^uint(0) >> 1)
		plan, err := item.store.PutReservationPlan(domain.ReservationPlan{Code: "PLAN-OVERFLOW", ApprovedReserved: maximum, RequestCeiling: maximum})
		if err != nil {
			t.Fatal(err)
		}
		reservation, err := item.store.PutStockReservation(domain.StockReservation{
			Code: "RES-OVERFLOW", StockID: item.stock.ID, PlanID: plan.ID, RequestedReserved: 0,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := item.store.CommitStockReservation(reservation.ID, staff, true); !errors.Is(err, domain.ErrNumericRepresentation) {
			t.Fatalf("representation error = %v", err)
		}
		storedReservation, _ := item.store.StockReservation(reservation.ID)
		storedStock, _ := item.store.StockItem(item.stock.ID)
		if storedReservation.Status != domain.ReservationPending || storedReservation.Version != reservation.Version || storedStock != item.stock {
			t.Fatalf("representation failure partially committed: reservation=%#v stock=%#v", storedReservation, storedStock)
		}
	})

	t.Run("invariant rejection preserves both entities", func(t *testing.T) {
		item := newFixture(t)
		plan, err := item.store.PutReservationPlan(domain.ReservationPlan{Code: "PLAN-INVALID", ApprovedReserved: item.stock.OnHand + 1, RequestCeiling: 6})
		if err != nil {
			t.Fatal(err)
		}
		reservation, err := item.store.PutStockReservation(domain.StockReservation{
			Code: "RES-INVALID", StockID: item.stock.ID, PlanID: plan.ID, RequestedReserved: 4,
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

	t.Run("precondition wins when predicate and invariant would both fail", func(t *testing.T) {
		item := newFixture(t)
		stock := item.stock
		stock.OnHand = 7
		if _, err := item.store.PutStockItem(stock); err != nil {
			t.Fatal(err)
		}
		plan := item.plan
		plan.RequestCeiling = 4
		if _, err := item.store.PutReservationPlan(plan); err != nil {
			t.Fatal(err)
		}
		if _, _, err := item.store.CommitStockReservation(item.reservation.ID, staff, true); !errors.Is(err, domain.ErrPrecondition) {
			t.Fatalf("combined rejection error = %v", err)
		}
		stored, _ := item.store.StockItem(item.stock.ID)
		if stored.Reserved != item.stock.Reserved {
			t.Fatalf("combined rejection changed stock = %#v", stored)
		}
	})

	t.Run("value unavailable preserves source and target", func(t *testing.T) {
		item := newFixture(t)
		item.store.RemoveReservationPlan(item.plan.ID)
		if _, _, err := item.store.CommitStockReservation(item.reservation.ID, staff, true); !errors.Is(err, domain.ErrValueUnavailable) {
			t.Fatalf("value-unavailable error = %v", err)
		}
		storedReservation, _ := item.store.StockReservation(item.reservation.ID)
		storedStock, _ := item.store.StockItem(item.stock.ID)
		if storedReservation.Status != domain.ReservationPending || storedReservation.Version != item.reservation.Version || storedStock != item.stock {
			t.Fatalf("value-unavailable partially committed: reservation=%#v stock=%#v", storedReservation, storedStock)
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

func TestConcurrentStockReservationCommitsRecheckPreconditionBeforeCommit(t *testing.T) {
	item := newFixture(t)
	firstPlan, err := item.store.PutReservationPlan(domain.ReservationPlan{Code: "PLAN-CONCURRENT-A", ApprovedReserved: 6, RequestCeiling: 6})
	if err != nil {
		t.Fatal(err)
	}
	secondPlan, err := item.store.PutReservationPlan(domain.ReservationPlan{Code: "PLAN-CONCURRENT-B", ApprovedReserved: 6, RequestCeiling: 6})
	if err != nil {
		t.Fatal(err)
	}
	first, err := item.store.PutStockReservation(domain.StockReservation{Code: "RES-CONCURRENT-A", StockID: item.stock.ID, PlanID: firstPlan.ID, RequestedReserved: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := item.store.PutStockReservation(domain.StockReservation{Code: "RES-CONCURRENT-B", StockID: item.stock.ID, PlanID: secondPlan.ID, RequestedReserved: 3})
	if err != nil {
		t.Fatal(err)
	}
	staff := domain.NewPrincipal(domain.RoleStaff)
	start := make(chan struct{})
	errorsByID := make(chan struct {
		id  string
		err error
	}, 2)
	for _, reservation := range []domain.StockReservation{first, second} {
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
	succeeded, rejected := 0, 0
	for _, err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, domain.ErrPrecondition) {
			rejected++
		} else {
			t.Fatalf("unexpected concurrent error = %v", err)
		}
	}
	storedFirst, _ := item.store.StockReservation(first.ID)
	storedSecond, _ := item.store.StockReservation(second.ID)
	stock, _ := item.store.StockItem(item.stock.ID)
	statuses := []domain.ReservationStatus{storedFirst.Status, storedSecond.Status}
	if succeeded != 1 || rejected != 1 || !slices.Contains(statuses, domain.ReservationCommitted) ||
		!slices.Contains(statuses, domain.ReservationPending) || stock.Reserved != 8 {
		t.Fatalf("concurrent atomic state = first %#v second %#v stock %#v results %#v", storedFirst, storedSecond, stock, results)
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
