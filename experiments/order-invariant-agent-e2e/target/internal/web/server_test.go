package web

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"example.com/forma-orders-target/internal/domain"
	"example.com/forma-orders-target/internal/store"
)

type webFixture struct {
	repository *store.Store
	handler    http.Handler
	customer   domain.Customer
	product    domain.Product
	order      domain.Order
	stock      domain.StockItem
}

func newWebFixture(t *testing.T) webFixture {
	t.Helper()
	repository := store.New()
	customer := repository.PutCustomer(domain.Customer{Name: "Aster Labs", Email: "orders@aster.example"})
	product := repository.PutProduct(domain.Product{SKU: "WIDGET-1", Name: "Widget", Price: "12.50"})
	stockItem, err := repository.PutStockItem(domain.StockItem{ProductID: product.ID, Location: "Tokyo", OnHand: 10, Reserved: 2})
	if err != nil {
		t.Fatal(err)
	}
	order, err := repository.CreateOrder(store.OrderInput{Number: "ORD-100", CustomerID: customer.ID}, "seed")
	if err != nil {
		t.Fatal(err)
	}
	return webFixture{
		repository: repository, handler: New(repository), customer: customer,
		product: product, order: order, stock: stockItem,
	}
}

func TestOrdersSurfaceListsFieldsActionsAndObservableFeedback(t *testing.T) {
	item := newWebFixture(t)
	response := request(t, item.handler, http.MethodGet, "/orders", "staff", nil)
	assertResponse(t, response, http.StatusOK, "data-fields=\"number customer status\"", "data-actions=\"create view edit delete submit approve reject ship\"", "ORD-100")

	empty := request(t, New(store.New()), http.MethodGet, "/orders", "admin", nil)
	assertResponse(t, empty, http.StatusOK, "empty")

	failing := &failingOrdersRepository{Store: item.repository}
	failure := request(t, New(failing), http.MethodGet, "/orders", "staff", nil)
	assertResponse(t, failure, http.StatusInternalServerError, "failure")
}

func TestOrdersListQueryHTTPBehavior(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		item := newWebFixture(t)
		if _, err := item.repository.CreateOrder(store.OrderInput{Number: "ORD-OTHER", CustomerID: item.customer.ID}, ""); err != nil {
			t.Fatal(err)
		}
		response := request(t, item.handler, http.MethodGet, "/orders?search=ord-100", "staff", nil)
		assertResponse(t, response, http.StatusOK, "ORD-100")
		assertResponseOmits(t, response, "ORD-OTHER")
	})

	t.Run("customer-filter", func(t *testing.T) {
		item := newWebFixture(t)
		other := item.repository.PutCustomer(domain.Customer{Name: "Beacon", Email: "orders@beacon.example"})
		if _, err := item.repository.CreateOrder(store.OrderInput{Number: "ORD-OTHER", CustomerID: other.ID}, ""); err != nil {
			t.Fatal(err)
		}
		response := request(t, item.handler, http.MethodGet, "/orders?customer="+url.QueryEscape(item.customer.ID), "staff", nil)
		assertResponse(t, response, http.StatusOK, "ORD-100")
		assertResponseOmits(t, response, "ORD-OTHER")
	})

	t.Run("status-filter", func(t *testing.T) {
		item := newWebFixture(t)
		submitted, err := item.repository.CreateOrder(store.OrderInput{Number: "ORD-SUBMITTED", CustomerID: item.customer.ID}, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := item.repository.TransitionOrder(submitted.ID, domain.ActionSubmit, domain.NewPrincipal(domain.RoleStaff), false); err != nil {
			t.Fatal(err)
		}
		response := request(t, item.handler, http.MethodGet, "/orders?status=Submitted", "admin", nil)
		assertResponse(t, response, http.StatusOK, "ORD-SUBMITTED")
		assertResponseOmits(t, response, "ORD-100")
	})

	t.Run("stable-sort", func(t *testing.T) {
		item := newWebFixture(t)
		for _, number := range []string{"ZZZ-ORDER", "AAA-ORDER"} {
			if _, err := item.repository.CreateOrder(store.OrderInput{Number: number, CustomerID: item.customer.ID}, ""); err != nil {
				t.Fatal(err)
			}
		}
		response := request(t, item.handler, http.MethodGet, "/orders", "staff", nil)
		assertResponseOrder(t, response, "AAA-ORDER", "ORD-100", "ZZZ-ORDER")
	})

	t.Run("page-boundary", func(t *testing.T) {
		item := newWebFixture(t)
		for index := 0; index < 20; index++ {
			if _, err := item.repository.CreateOrder(store.OrderInput{
				Number: fmt.Sprintf("PAGE-%02d", index), CustomerID: item.customer.ID,
			}, ""); err != nil {
				t.Fatal(err)
			}
		}
		first := request(t, item.handler, http.MethodGet, "/orders", "admin", nil)
		if count := strings.Count(first.Body.String(), "<article data-id="); count != 20 {
			t.Fatalf("first page records = %d, want 20", count)
		}
		assertResponse(t, first, http.StatusOK, "rel=\"next\" href=\"/orders?page=2\"")
		assertResponseOmits(t, first, "PAGE-19")

		second := request(t, item.handler, http.MethodGet, "/orders?page=2", "admin", nil)
		if count := strings.Count(second.Body.String(), "<article data-id="); count != 1 {
			t.Fatalf("second page records = %d, want 1; body=%s", count, second.Body.String())
		}
		assertResponse(t, second, http.StatusOK, "PAGE-19")
		assertResponseOmits(t, second, "rel=\"next\"")
	})
}

func TestOrderCreateSurfaceValidationMutationAndNavigation(t *testing.T) {
	item := newWebFixture(t)
	form := request(t, item.handler, http.MethodGet, "/orders/new", "staff", nil)
	assertResponse(t, form, http.StatusOK, "name=\"number\"", "name=\"customer\"")

	invalid := request(t, item.handler, http.MethodPost, "/orders", "staff", url.Values{
		"number": {""}, "customer": {item.customer.ID}, "_submission": {"invalid"},
	})
	assertResponse(t, invalid, http.StatusUnprocessableEntity, "invalid", "name=\"customer\" value=\""+item.customer.ID+"\"")

	created := request(t, item.handler, http.MethodPost, "/orders", "staff", url.Values{
		"number": {"ORD-200"}, "customer": {item.customer.ID}, "_submission": {"create"},
	})
	if created.Code != http.StatusSeeOther || !strings.HasPrefix(created.Header().Get("Location"), "/orders/order-") {
		t.Fatalf("create response = %d location=%q body=%s", created.Code, created.Header().Get("Location"), created.Body.String())
	}
	createdID := strings.TrimPrefix(created.Header().Get("Location"), "/orders/")
	storedCreated, err := item.repository.Order(createdID)
	if err != nil || storedCreated.Number != "ORD-200" || storedCreated.CustomerID != item.customer.ID {
		t.Fatalf("created order = %#v, %v", storedCreated, err)
	}
	replayed := request(t, item.handler, http.MethodPost, "/orders", "staff", url.Values{
		"number": {"IGNORED"}, "customer": {item.customer.ID}, "_submission": {"create"},
	})
	if replayed.Header().Get("Location") != created.Header().Get("Location") {
		t.Fatalf("repeated create moved to %q, want %q", replayed.Header().Get("Location"), created.Header().Get("Location"))
	}
	orders, _ := item.repository.Orders(store.OrderQuery{})
	if len(orders.Items) != 2 {
		t.Fatalf("repeated HTTP create produced %d orders, want 2 total", len(orders.Items))
	}
	failure := request(t, New(&failingCreateOrderRepository{Store: item.repository}), http.MethodPost, "/orders", "staff", url.Values{
		"number": {"ORD-FAIL"}, "customer": {item.customer.ID}, "_submission": {"failure"},
	})
	assertResponse(t, failure, http.StatusInternalServerError, "failure", "name=\"number\" value=\"ORD-FAIL\"")
}

func TestOrderDetailSurfaceFieldsActionsEmptyAndFailure(t *testing.T) {
	item := newWebFixture(t)
	detail := request(t, item.handler, http.MethodGet, "/orders/"+item.order.ID, "admin", nil)
	assertResponse(t, detail, http.StatusOK, "data-fields=\"number customer status\"", "data-actions=\"edit delete submit approve reject ship\"", "ORD-100")

	empty := request(t, item.handler, http.MethodGet, "/orders/missing", "staff", nil)
	assertResponse(t, empty, http.StatusNotFound, "empty")

	failing := &failingOrderRepository{Store: item.repository}
	failure := request(t, New(failing), http.MethodGet, "/orders/"+item.order.ID, "admin", nil)
	assertResponse(t, failure, http.StatusInternalServerError, "failure")
}

func TestOrderEditSurfaceValidationMutationAndNavigation(t *testing.T) {
	item := newWebFixture(t)
	form := request(t, item.handler, http.MethodGet, "/orders/"+item.order.ID+"/edit", "staff", nil)
	assertResponse(t, form, http.StatusOK, "name=\"number\" value=\"ORD-100\"", "name=\"customer\"")

	invalid := request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID, "staff", url.Values{
		"number": {""}, "customer": {item.customer.ID}, "_submission": {"invalid"},
	})
	assertResponse(t, invalid, http.StatusUnprocessableEntity, "invalid", "name=\"customer\" value=\""+item.customer.ID+"\"")

	updated := request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID, "staff", url.Values{
		"number": {"ORD-101"}, "customer": {item.customer.ID}, "_submission": {"update"},
	})
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != "/orders/"+item.order.ID {
		t.Fatalf("update response = %d location=%q", updated.Code, updated.Header().Get("Location"))
	}
	storedUpdated, err := item.repository.Order(item.order.ID)
	if err != nil || storedUpdated.Number != "ORD-101" {
		t.Fatalf("updated order = %#v, %v", storedUpdated, err)
	}
	replayed := request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID, "staff", url.Values{
		"number": {"IGNORED"}, "customer": {item.customer.ID}, "_submission": {"update"},
	})
	if replayed.Header().Get("Location") != updated.Header().Get("Location") {
		t.Fatalf("repeated update moved to %q, want %q", replayed.Header().Get("Location"), updated.Header().Get("Location"))
	}
	storedReplayed, _ := item.repository.Order(item.order.ID)
	if storedReplayed.Number != storedUpdated.Number || storedReplayed.Version != storedUpdated.Version {
		t.Fatalf("repeated HTTP edit applied again: first=%#v replay=%#v", storedUpdated, storedReplayed)
	}
	failure := request(t, New(&failingUpdateOrderRepository{Store: item.repository}), http.MethodPost, "/orders/"+item.order.ID, "staff", url.Values{
		"number": {"ORD-FAIL"}, "customer": {item.customer.ID}, "_submission": {"failure"},
	})
	assertResponse(t, failure, http.StatusInternalServerError, "failure", "name=\"number\" value=\"ORD-FAIL\"")
}

func TestOrderActionNavigationFromListAndDetail(t *testing.T) {
	item := newWebFixture(t)
	list := request(t, item.handler, http.MethodGet, "/orders", "staff", nil)
	assertResponse(t, list, http.StatusOK,
		"href=\"/orders/new\"", "href=\"/orders/"+item.order.ID+"\"", "href=\"/orders/"+item.order.ID+"/edit\"",
		"action=\"/orders/"+item.order.ID+"/delete?from=list\"",
		"action=\"/orders/"+item.order.ID+"/actions/submit?from=list\"",
		"action=\"/orders/"+item.order.ID+"/actions/approve?from=list\"",
		"action=\"/orders/"+item.order.ID+"/actions/reject?from=list\"",
		"action=\"/orders/"+item.order.ID+"/actions/ship?from=list\"",
	)
	detail := request(t, item.handler, http.MethodGet, "/orders/"+item.order.ID, "staff", nil)
	assertResponse(t, detail, http.StatusOK, "href=\"/orders/"+item.order.ID+"/edit\"",
		"action=\"/orders/"+item.order.ID+"/delete\"",
		"action=\"/orders/"+item.order.ID+"/actions/submit\"",
		"action=\"/orders/"+item.order.ID+"/actions/approve\"",
		"action=\"/orders/"+item.order.ID+"/actions/reject\"",
		"action=\"/orders/"+item.order.ID+"/actions/ship\"",
	)

	submitted := request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID+"/actions/submit?from=list", "staff", nil)
	if submitted.Code != http.StatusSeeOther || submitted.Header().Get("Location") != "/orders" {
		t.Fatalf("list transition = %d location=%q", submitted.Code, submitted.Header().Get("Location"))
	}
	approved := request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID+"/actions/approve?confirmed=true", "admin", nil)
	if approved.Code != http.StatusSeeOther || approved.Header().Get("Location") != "/orders/"+item.order.ID {
		t.Fatalf("detail transition = %d location=%q", approved.Code, approved.Header().Get("Location"))
	}
	shipped := request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID+"/actions/ship", "staff", nil)
	if shipped.Code != http.StatusSeeOther || shipped.Header().Get("Location") != "/orders/"+item.order.ID {
		t.Fatalf("ship transition = %d location=%q", shipped.Code, shipped.Header().Get("Location"))
	}
	deleted := request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID+"/delete", "admin", nil)
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != "/orders" {
		t.Fatalf("delete = %d location=%q", deleted.Code, deleted.Header().Get("Location"))
	}

	assertOrderActionRedirects(t, item)
}

func TestStockItemsSurfaceListsFieldsActionsAndObservableFeedback(t *testing.T) {
	item := newWebFixture(t)
	response := request(t, item.handler, http.MethodGet, "/stock-items", "staff", nil)
	assertResponse(t, response, http.StatusOK, "data-fields=\"location product onHand reserved\"", "data-actions=\"view edit\"", "Tokyo")

	empty := request(t, New(store.New()), http.MethodGet, "/stock-items", "admin", nil)
	assertResponse(t, empty, http.StatusOK, "empty")

	failing := &failingStockItemsRepository{Store: item.repository}
	failure := request(t, New(failing), http.MethodGet, "/stock-items", "staff", nil)
	assertResponse(t, failure, http.StatusInternalServerError, "failure")
}

func TestStockItemsListQueryHTTPBehavior(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		item := newWebFixture(t)
		if _, err := item.repository.PutStockItem(domain.StockItem{ProductID: item.product.ID, Location: "Osaka", OnHand: 5}); err != nil {
			t.Fatal(err)
		}
		response := request(t, item.handler, http.MethodGet, "/stock-items?search=tok", "staff", nil)
		assertResponse(t, response, http.StatusOK, "Tokyo")
		assertResponseOmits(t, response, "Osaka")
	})

	t.Run("product-filter", func(t *testing.T) {
		item := newWebFixture(t)
		other := item.repository.PutProduct(domain.Product{SKU: "OTHER", Name: "Other", Price: "1.00"})
		if _, err := item.repository.PutStockItem(domain.StockItem{ProductID: other.ID, Location: "Osaka", OnHand: 5}); err != nil {
			t.Fatal(err)
		}
		response := request(t, item.handler, http.MethodGet, "/stock-items?product="+url.QueryEscape(item.product.ID), "admin", nil)
		assertResponse(t, response, http.StatusOK, "Tokyo")
		assertResponseOmits(t, response, "Osaka")
	})

	t.Run("stable-sort", func(t *testing.T) {
		item := newWebFixture(t)
		for _, location := range []string{"Zulu", "Aichi"} {
			if _, err := item.repository.PutStockItem(domain.StockItem{ProductID: item.product.ID, Location: location, OnHand: 5}); err != nil {
				t.Fatal(err)
			}
		}
		response := request(t, item.handler, http.MethodGet, "/stock-items", "staff", nil)
		assertResponseOrder(t, response, "Aichi", "Tokyo", "Zulu")
	})

	t.Run("page-boundary", func(t *testing.T) {
		item := newWebFixture(t)
		for index := 0; index < 20; index++ {
			if _, err := item.repository.PutStockItem(domain.StockItem{
				ProductID: item.product.ID, Location: fmt.Sprintf("Warehouse-%02d", index), OnHand: 5,
			}); err != nil {
				t.Fatal(err)
			}
		}
		first := request(t, item.handler, http.MethodGet, "/stock-items", "staff", nil)
		if count := strings.Count(first.Body.String(), "<article data-id="); count != 20 {
			t.Fatalf("first page records = %d, want 20", count)
		}
		assertResponse(t, first, http.StatusOK, "rel=\"next\" href=\"/stock-items?page=2\"")
		assertResponseOmits(t, first, "Warehouse-19")

		second := request(t, item.handler, http.MethodGet, "/stock-items?page=2", "staff", nil)
		if count := strings.Count(second.Body.String(), "<article data-id="); count != 1 {
			t.Fatalf("second page records = %d, want 1; body=%s", count, second.Body.String())
		}
		assertResponse(t, second, http.StatusOK, "Warehouse-19")
		assertResponseOmits(t, second, "rel=\"next\"")
	})
}

func TestStockItemDetailSurfaceFieldsActionsEmptyAndFailure(t *testing.T) {
	item := newWebFixture(t)
	detail := request(t, item.handler, http.MethodGet, "/stock-items/"+item.stock.ID, "staff", nil)
	assertResponse(t, detail, http.StatusOK, "data-fields=\"location product onHand reserved\"", "data-actions=\"edit\"", "Tokyo")

	empty := request(t, item.handler, http.MethodGet, "/stock-items/missing", "admin", nil)
	assertResponse(t, empty, http.StatusNotFound, "empty")

	failing := &failingStockItemRepository{Store: item.repository}
	failure := request(t, New(failing), http.MethodGet, "/stock-items/"+item.stock.ID, "staff", nil)
	assertResponse(t, failure, http.StatusInternalServerError, "failure")
}

func TestStockItemEditSurfaceValidationInvariantAndNavigation(t *testing.T) {
	item := newWebFixture(t)
	form := request(t, item.handler, http.MethodGet, "/stock-items/"+item.stock.ID+"/edit", "admin", nil)
	assertResponse(t, form, http.StatusOK, "name=\"location\"", "name=\"product\"", "name=\"onHand\"", "name=\"reserved\"")

	invalidInputs := []url.Values{
		{"location": {""}, "product": {item.product.ID}, "onHand": {"10"}, "reserved": {"2"}, "_submission": {"location-required"}},
		{"location": {"Tokyo"}, "product": {item.product.ID}, "onHand": {""}, "reserved": {"2"}, "_submission": {"on-hand-required"}},
		{"location": {"Tokyo"}, "product": {item.product.ID}, "onHand": {"10"}, "reserved": {""}, "_submission": {"reserved-required"}},
		{"location": {"Tokyo"}, "product": {item.product.ID}, "onHand": {"-1"}, "reserved": {"0"}, "_submission": {"on-hand-min"}},
		{"location": {"Tokyo"}, "product": {item.product.ID}, "onHand": {"10"}, "reserved": {"-1"}, "_submission": {"reserved-min"}},
		{"location": {"Tokyo"}, "product": {item.product.ID}, "onHand": {"1"}, "reserved": {"5"}, "_submission": {"invariant"}},
	}
	for _, input := range invalidInputs {
		invalid := request(t, item.handler, http.MethodPost, "/stock-items/"+item.stock.ID, "admin", input)
		assertResponse(t, invalid, http.StatusUnprocessableEntity, "invalid",
			"name=\"location\" value=\""+html.EscapeString(input.Get("location"))+"\"",
			"name=\"product\" value=\""+html.EscapeString(input.Get("product"))+"\"",
			"name=\"onHand\" value=\""+html.EscapeString(input.Get("onHand"))+"\"",
			"name=\"reserved\" value=\""+html.EscapeString(input.Get("reserved"))+"\"",
		)
		stored, _ := item.repository.StockItem(item.stock.ID)
		if stored != item.stock {
			t.Fatalf("invalid HTTP mutation changed stock: got %#v want %#v", stored, item.stock)
		}
	}

	updated := request(t, item.handler, http.MethodPost, "/stock-items/"+item.stock.ID, "admin", url.Values{
		"location": {"Osaka"}, "product": {item.product.ID}, "onHand": {"12"}, "reserved": {"4"}, "_submission": {"update"},
	})
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != "/stock-items/"+item.stock.ID {
		t.Fatalf("stock update = %d location=%q", updated.Code, updated.Header().Get("Location"))
	}
	storedUpdated, err := item.repository.StockItem(item.stock.ID)
	if err != nil || storedUpdated.Location != "Osaka" || storedUpdated.OnHand != 12 || storedUpdated.Reserved != 4 {
		t.Fatalf("updated stock = %#v, %v", storedUpdated, err)
	}
	replayed := request(t, item.handler, http.MethodPost, "/stock-items/"+item.stock.ID, "admin", url.Values{
		"location": {"Ignored"}, "product": {item.product.ID}, "onHand": {"1"}, "reserved": {"0"}, "_submission": {"update"},
	})
	if replayed.Header().Get("Location") != updated.Header().Get("Location") {
		t.Fatalf("repeated stock update moved to %q, want %q", replayed.Header().Get("Location"), updated.Header().Get("Location"))
	}
	storedReplayed, _ := item.repository.StockItem(item.stock.ID)
	if storedReplayed != storedUpdated {
		t.Fatalf("repeated HTTP stock edit applied again: first=%#v replay=%#v", storedUpdated, storedReplayed)
	}
	failure := request(t, New(&failingUpdateStockRepository{Store: item.repository}), http.MethodPost, "/stock-items/"+item.stock.ID, "admin", url.Values{
		"location": {"Failure"}, "product": {item.product.ID}, "onHand": {"10"}, "reserved": {"2"}, "_submission": {"failure"},
	})
	assertResponse(t, failure, http.StatusInternalServerError, "failure", "name=\"location\" value=\"Failure\"")
}

func TestStockActionNavigationFromListAndDetail(t *testing.T) {
	item := newWebFixture(t)
	list := request(t, item.handler, http.MethodGet, "/stock-items", "staff", nil)
	assertResponse(t, list, http.StatusOK,
		"href=\"/stock-items/"+item.stock.ID+"\"", "href=\"/stock-items/"+item.stock.ID+"/edit\"",
	)
	detail := request(t, item.handler, http.MethodGet, "/stock-items/"+item.stock.ID, "admin", nil)
	assertResponse(t, detail, http.StatusOK, "href=\"/stock-items/"+item.stock.ID+"/edit\"")
}

func TestOrdersHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "list", authenticatedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/orders", roles, nil)
	})
}

func TestOrderCreateActionHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "list-action", staffWithCombinedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/orders/new?from=list", roles, nil)
	})
}

func TestOrderCreateFormHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "form", staffCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/orders/new", roles, nil)
	})
}

func TestOrderCreateSubmitHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "submit", staffWithCombinedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodPost, "/orders", roles, url.Values{
			"number": {"ACCESS-CREATE"}, "customer": {item.customer.ID}, "_submission": {"access-create"},
		})
	})
}

func TestOrderDetailActionHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "list-action", authenticatedWithCombinedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/orders/"+item.order.ID+"?from=list", roles, nil)
	})
}

func TestOrderDetailHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "detail", authenticatedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/orders/"+item.order.ID, roles, nil)
	})
}

func TestOrderEditActionsHTTPAccess(t *testing.T) {
	for _, surface := range []string{"list", "detail"} {
		surface := surface
		assertHTTPAccess(t, surface+"-action", staffWithCombinedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
			return request(t, item.handler, http.MethodGet, "/orders/"+item.order.ID+"/edit?from="+surface, roles, nil)
		})
	}
}

func TestOrderEditFormHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "form", staffCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/orders/"+item.order.ID+"/edit", roles, nil)
	})
}

func TestOrderEditSubmitHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "submit", staffWithCombinedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID, roles, url.Values{
			"number": {"ACCESS-EDIT"}, "customer": {item.customer.ID}, "_submission": {"access-edit"},
		})
	})
}

func TestOrderDeleteActionsHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "list-action", authenticatedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID+"/delete?from=list", roles, nil)
	})
	assertHTTPAccess(t, "detail-action", authenticatedWithCombinedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodPost, "/orders/"+item.order.ID+"/delete", roles, nil)
	})
}

func TestOrderSubmitActionsHTTPAccess(t *testing.T) {
	for _, surface := range []string{"list", "detail"} {
		surface := surface
		assertHTTPAccess(t, surface+"-action", authenticatedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
			return orderActionAccessRequest(t, item, domain.ActionSubmit, surface, roles)
		})
	}
}

func TestOrderApproveActionsHTTPAccess(t *testing.T) {
	for _, surface := range []string{"list", "detail"} {
		surface := surface
		assertHTTPAccess(t, surface+"-action", adminWithCombinedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
			return orderActionAccessRequest(t, item, domain.ActionApprove, surface, roles)
		})
	}
}

func TestOrderRejectActionsHTTPAccess(t *testing.T) {
	for _, surface := range []string{"list", "detail"} {
		surface := surface
		assertHTTPAccess(t, surface+"-action", adminWithCombinedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
			return orderActionAccessRequest(t, item, domain.ActionReject, surface, roles)
		})
	}
}

func TestOrderShipActionsHTTPAccess(t *testing.T) {
	for _, surface := range []string{"list", "detail"} {
		surface := surface
		assertHTTPAccess(t, surface+"-action", staffWithCombinedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
			return orderActionAccessRequest(t, item, domain.ActionShip, surface, roles)
		})
	}
}

func TestStockItemsHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "list", authenticatedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/stock-items", roles, nil)
	})
}

func TestStockItemDetailActionHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "list-action", authenticatedWithCombinedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/stock-items/"+item.stock.ID+"?from=list", roles, nil)
	})
}

func TestStockItemDetailHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "detail", authenticatedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/stock-items/"+item.stock.ID, roles, nil)
	})
}

func TestStockItemEditActionsHTTPAccess(t *testing.T) {
	for _, surface := range []string{"list", "detail"} {
		surface := surface
		assertHTTPAccess(t, surface+"-action", adminWithCombinedCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
			return request(t, item.handler, http.MethodGet, "/stock-items/"+item.stock.ID+"/edit?from="+surface, roles, nil)
		})
	}
}

func TestStockItemEditFormHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "form", adminCases, http.StatusOK, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodGet, "/stock-items/"+item.stock.ID+"/edit", roles, nil)
	})
}

func TestStockItemEditSubmitHTTPAccess(t *testing.T) {
	assertHTTPAccess(t, "submit", adminWithCombinedCases, http.StatusSeeOther, func(t *testing.T, item webFixture, roles string) *httptest.ResponseRecorder {
		return request(t, item.handler, http.MethodPost, "/stock-items/"+item.stock.ID, roles, url.Values{
			"location": {"Access"}, "product": {item.product.ID}, "onHand": {"10"}, "reserved": {"2"}, "_submission": {"access-stock"},
		})
	})
}

type accessExpectation struct {
	name    string
	roles   string
	allowed bool
}

var (
	authenticatedCases = []accessExpectation{
		{name: "admin", roles: "admin", allowed: true},
		{name: "staff", roles: "staff", allowed: true},
		{name: "anonymous"},
	}
	authenticatedWithCombinedCases = []accessExpectation{
		{name: "admin", roles: "admin", allowed: true},
		{name: "staff", roles: "staff", allowed: true},
		{name: "admin+staff", roles: "admin,staff", allowed: true},
		{name: "anonymous"},
	}
	staffCases = []accessExpectation{
		{name: "admin", roles: "admin"},
		{name: "staff", roles: "staff", allowed: true},
		{name: "anonymous"},
	}
	staffWithCombinedCases = []accessExpectation{
		{name: "admin", roles: "admin"},
		{name: "staff", roles: "staff", allowed: true},
		{name: "admin+staff", roles: "admin,staff", allowed: true},
		{name: "anonymous"},
	}
	adminCases = []accessExpectation{
		{name: "admin", roles: "admin", allowed: true},
		{name: "staff", roles: "staff"},
		{name: "anonymous"},
	}
	adminWithCombinedCases = []accessExpectation{
		{name: "admin", roles: "admin", allowed: true},
		{name: "staff", roles: "staff"},
		{name: "admin+staff", roles: "admin,staff", allowed: true},
		{name: "anonymous"},
	}
)

func assertHTTPAccess(
	t *testing.T,
	surface string,
	cases []accessExpectation,
	allowedStatus int,
	perform func(*testing.T, webFixture, string) *httptest.ResponseRecorder,
) {
	t.Helper()
	for _, test := range cases {
		test := test
		t.Run(surface+"/"+test.name, func(t *testing.T) {
			item := newWebFixture(t)
			response := perform(t, item, test.roles)
			want := http.StatusForbidden
			if test.allowed {
				want = allowedStatus
			}
			if response.Code != want {
				t.Fatalf("roles %q = %d, want %d; body=%s", test.roles, response.Code, want, response.Body.String())
			}
		})
	}
}

func orderActionAccessRequest(t *testing.T, item webFixture, action domain.Action, surface, roles string) *httptest.ResponseRecorder {
	t.Helper()
	switch action {
	case domain.ActionApprove, domain.ActionReject:
		if _, err := item.repository.TransitionOrder(item.order.ID, domain.ActionSubmit, domain.NewPrincipal(domain.RoleStaff), false); err != nil {
			t.Fatal(err)
		}
	case domain.ActionShip:
		if _, err := item.repository.TransitionOrder(item.order.ID, domain.ActionSubmit, domain.NewPrincipal(domain.RoleStaff), false); err != nil {
			t.Fatal(err)
		}
		if _, err := item.repository.TransitionOrder(item.order.ID, domain.ActionApprove, domain.NewPrincipal(domain.RoleAdmin), true); err != nil {
			t.Fatal(err)
		}
	}
	target := "/orders/" + item.order.ID + "/actions/" + string(action) + "?from=" + surface
	if action == domain.ActionApprove {
		target += "&confirmed=true"
	}
	return request(t, item.handler, http.MethodPost, target, roles, nil)
}

type failingOrdersRepository struct{ *store.Store }

func (repository *failingOrdersRepository) Orders(store.OrderQuery) (store.Page[domain.Order], error) {
	return store.Page[domain.Order]{}, domain.ErrUnavailable
}

type failingOrderRepository struct{ *store.Store }

func (repository *failingOrderRepository) Order(string) (domain.Order, error) {
	return domain.Order{}, domain.ErrUnavailable
}

type failingCreateOrderRepository struct{ *store.Store }

func (repository *failingCreateOrderRepository) CreateOrder(store.OrderInput, string) (domain.Order, error) {
	return domain.Order{}, domain.ErrUnavailable
}

type failingUpdateOrderRepository struct{ *store.Store }

func (repository *failingUpdateOrderRepository) UpdateOrder(string, store.OrderInput, string) (domain.Order, error) {
	return domain.Order{}, domain.ErrUnavailable
}

type failingStockItemsRepository struct{ *store.Store }

func (repository *failingStockItemsRepository) StockItems(store.StockQuery) (store.Page[domain.StockItem], error) {
	return store.Page[domain.StockItem]{}, domain.ErrUnavailable
}

type failingStockItemRepository struct{ *store.Store }

func (repository *failingStockItemRepository) StockItem(string) (domain.StockItem, error) {
	return domain.StockItem{}, domain.ErrUnavailable
}

type failingUpdateStockRepository struct{ *store.Store }

func (repository *failingUpdateStockRepository) UpdateStockItem(string, store.StockInput, string) (domain.StockItem, error) {
	return domain.StockItem{}, domain.ErrUnavailable
}

func assertOrderActionRedirects(t *testing.T, item webFixture) {
	t.Helper()
	makeOrder := func(number string, status domain.OrderStatus) domain.Order {
		order, err := item.repository.CreateOrder(store.OrderInput{Number: number, CustomerID: item.customer.ID}, "")
		if err != nil {
			t.Fatal(err)
		}
		if status == domain.OrderSubmitted || status == domain.OrderApproved {
			order, err = item.repository.TransitionOrder(order.ID, domain.ActionSubmit, domain.NewPrincipal(domain.RoleStaff), false)
			if err != nil {
				t.Fatal(err)
			}
		}
		if status == domain.OrderApproved {
			order, err = item.repository.TransitionOrder(order.ID, domain.ActionApprove, domain.NewPrincipal(domain.RoleAdmin), true)
			if err != nil {
				t.Fatal(err)
			}
		}
		return order
	}
	cases := []struct {
		name   string
		status domain.OrderStatus
		action string
		roles  string
		extra  string
	}{
		{name: "submit", status: domain.OrderDraft, action: "submit", roles: "staff"},
		{name: "approve", status: domain.OrderSubmitted, action: "approve", roles: "admin", extra: "&confirmed=true"},
		{name: "reject", status: domain.OrderSubmitted, action: "reject", roles: "admin"},
		{name: "ship", status: domain.OrderApproved, action: "ship", roles: "staff"},
	}
	for index, test := range cases {
		for _, fromList := range []bool{false, true} {
			order := makeOrder(fmt.Sprintf("NAV-%s-%d-%t", test.name, index, fromList), test.status)
			query := "?from=detail"
			want := "/orders/" + order.ID
			if fromList {
				query = "?from=list"
				want = "/orders"
			}
			query += test.extra
			response := request(t, item.handler, http.MethodPost, "/orders/"+order.ID+"/actions/"+test.action+query, test.roles, nil)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != want {
				t.Errorf("%s fromList=%t = %d location=%q, want %q", test.name, fromList, response.Code, response.Header().Get("Location"), want)
			}
		}
	}
}

func request(t *testing.T, handler http.Handler, method, target, roles string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if roles != "" {
		req.Header.Set("X-Roles", roles)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func assertResponse(t *testing.T, response *httptest.ResponseRecorder, status int, values ...string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, status, response.Body.String())
	}
	for _, value := range values {
		if !strings.Contains(response.Body.String(), value) {
			t.Errorf("response omits %q:\n%s", value, response.Body.String())
		}
	}
}

func assertResponseOmits(t *testing.T, response *httptest.ResponseRecorder, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(response.Body.String(), value) {
			t.Errorf("response unexpectedly contains %q:\n%s", value, response.Body.String())
		}
	}
}

func assertResponseOrder(t *testing.T, response *httptest.ResponseRecorder, values ...string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	previous := -1
	for _, value := range values {
		index := strings.Index(response.Body.String(), value)
		if index < 0 {
			t.Fatalf("response omits %q:\n%s", value, response.Body.String())
		}
		if index <= previous {
			t.Fatalf("response does not order %v ascending:\n%s", values, response.Body.String())
		}
		previous = index
	}
}

func ExampleServer() {
	repository := store.New()
	server := New(repository)
	response := requestForExample(server, "/orders", "staff")
	fmt.Println(response.Code)
	// Output: 200
}

func requestForExample(handler http.Handler, target, roles string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("X-Roles", roles)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
