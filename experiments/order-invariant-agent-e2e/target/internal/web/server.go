package web

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"example.com/forma-orders-target/internal/domain"
	"example.com/forma-orders-target/internal/store"
)

type Repository interface {
	Orders(store.OrderQuery) (store.Page[domain.Order], error)
	Order(string) (domain.Order, error)
	CreateOrder(store.OrderInput, string) (domain.Order, error)
	UpdateOrder(string, store.OrderInput, string) (domain.Order, error)
	DeleteOrder(string, domain.Principal) error
	TransitionOrder(string, domain.Action, domain.Principal, bool) (domain.Order, error)
	StockItems(store.StockQuery) (store.Page[domain.StockItem], error)
	StockItem(string) (domain.StockItem, error)
	UpdateStockItem(string, store.StockInput, string) (domain.StockItem, error)
	StockReservations() ([]domain.StockReservation, error)
	StockReservation(string) (domain.StockReservation, error)
	CommitStockReservation(string, domain.Principal, bool) (domain.StockReservation, domain.StockItem, error)
}

type Server struct {
	repository Repository
	mux        *http.ServeMux
	tokens     atomic.Uint64
}

func New(repository Repository) *Server {
	server := &Server{repository: repository, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	server.mux.ServeHTTP(writer, request)
}

func (server *Server) routes() {
	server.mux.HandleFunc("GET /", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/orders", http.StatusSeeOther)
	})
	server.mux.HandleFunc("GET /orders", server.orders)
	server.mux.HandleFunc("GET /orders/new", server.orderCreate)
	server.mux.HandleFunc("POST /orders", server.createOrder)
	server.mux.HandleFunc("GET /orders/{id}", server.orderDetail)
	server.mux.HandleFunc("GET /orders/{id}/edit", server.orderEdit)
	server.mux.HandleFunc("POST /orders/{id}", server.updateOrder)
	server.mux.HandleFunc("POST /orders/{id}/delete", server.deleteOrder)
	server.mux.HandleFunc("POST /orders/{id}/actions/{action}", server.transitionOrder)
	server.mux.HandleFunc("GET /stock-items", server.stockItems)
	server.mux.HandleFunc("GET /stock-items/{id}", server.stockItemDetail)
	server.mux.HandleFunc("GET /stock-items/{id}/edit", server.stockItemEdit)
	server.mux.HandleFunc("POST /stock-items/{id}", server.updateStockItem)
	server.mux.HandleFunc("GET /reservations", server.reservations)
	server.mux.HandleFunc("POST /reservations/{id}/actions/commit", server.commitReservation)
}

func (server *Server) orders(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.OrdersView) {
		return
	}
	parameters := request.URL.Query()
	pageNumber := requestedPage(parameters.Get("page"))
	page, err := server.repository.Orders(store.OrderQuery{
		Search: parameters.Get("search"), Status: domain.OrderStatus(parameters.Get("status")),
		CustomerID: parameters.Get("customer"), Page: pageNumber,
	})
	if err != nil {
		feedback(writer, http.StatusInternalServerError, "failure", nil)
		return
	}
	var body strings.Builder
	body.WriteString("<h1>Orders</h1><div data-fields=\"number customer status\"></div><a data-action=\"create\" href=\"/orders/new\">create</a>")
	body.WriteString("<div data-actions=\"create view edit delete submit approve reject ship\"></div>")
	body.WriteString("<form><input name=\"search\"><select name=\"status\"></select><select name=\"customer\"></select></form>")
	if len(page.Items) == 0 {
		body.WriteString("<p role=status>empty</p>")
	}
	for _, order := range page.Items {
		id := html.EscapeString(order.ID)
		fmt.Fprintf(&body, "<article data-id=\"%s\"><a data-action=\"view\" href=\"/orders/%s\">%s</a><a data-action=\"edit\" href=\"/orders/%s/edit\">edit</a>", id, id, html.EscapeString(order.Number), id)
		for _, action := range []string{"delete", "submit", "approve", "reject", "ship"} {
			fmt.Fprintf(&body, "<form data-action=\"%s\"%s action=\"/orders/%s/%s%s\"></form>", action, confirmationAttribute(action), id, actionPath(action), actionQuery(action, "list"))
		}
		body.WriteString("</article>")
	}
	if page.HasMore {
		parameters.Set("page", strconv.Itoa(pageNumber+1))
		fmt.Fprintf(&body, "<a rel=\"next\" href=\"/orders?%s\">next</a>", html.EscapeString(parameters.Encode()))
	}
	respond(writer, http.StatusOK, body.String())
}

func (server *Server) orderCreate(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.OrderCreate) {
		return
	}
	respond(writer, http.StatusOK, orderForm("", "", "", server.nextToken()))
}

func (server *Server) createOrder(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.OrderCreate) {
		return
	}
	if err := request.ParseForm(); err != nil {
		feedback(writer, http.StatusUnprocessableEntity, "invalid", nil)
		return
	}
	input := store.OrderInput{Number: request.Form.Get("number"), CustomerID: request.Form.Get("customer")}
	order, err := server.repository.CreateOrder(input, request.Form.Get("_submission"))
	if err != nil {
		server.mutationError(writer, err, map[string]string{"number": input.Number, "customer": input.CustomerID})
		return
	}
	http.Redirect(writer, request, "/orders/"+order.ID, http.StatusSeeOther)
}

func (server *Server) orderDetail(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.OrderDetail) {
		return
	}
	order, err := server.repository.Order(request.PathValue("id"))
	if err != nil {
		server.readError(writer, err)
		return
	}
	id := html.EscapeString(order.ID)
	body := fmt.Sprintf(
		"<h1>%s</h1><div data-fields=\"number customer status\"></div><div data-actions=\"edit delete submit approve reject ship\"></div><a data-action=\"edit\" href=\"/orders/%s/edit\">edit</a>",
		html.EscapeString(order.Number), id,
	)
	for _, action := range []string{"delete", "submit", "approve", "reject", "ship"} {
		body += fmt.Sprintf("<form data-action=\"%s\"%s action=\"/orders/%s/%s%s\"></form>", action, confirmationAttribute(action), id, actionPath(action), actionQuery(action, ""))
	}
	respond(writer, http.StatusOK, body)
}

func (server *Server) orderEdit(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.OrderEdit) {
		return
	}
	order, err := server.repository.Order(request.PathValue("id"))
	if err != nil {
		server.readError(writer, err)
		return
	}
	respond(writer, http.StatusOK, orderForm(order.ID, order.Number, order.CustomerID, server.nextToken()))
}

func (server *Server) updateOrder(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.OrderEdit) {
		return
	}
	if err := request.ParseForm(); err != nil {
		feedback(writer, http.StatusUnprocessableEntity, "invalid", nil)
		return
	}
	input := store.OrderInput{Number: request.Form.Get("number"), CustomerID: request.Form.Get("customer")}
	order, err := server.repository.UpdateOrder(request.PathValue("id"), input, request.Form.Get("_submission"))
	if err != nil {
		server.mutationError(writer, err, map[string]string{"number": input.Number, "customer": input.CustomerID})
		return
	}
	http.Redirect(writer, request, "/orders/"+order.ID, http.StatusSeeOther)
}

func (server *Server) deleteOrder(writer http.ResponseWriter, request *http.Request) {
	if !domain.Allowed(principal(request), domain.OrderDelete) {
		server.actionError(writer, domain.ErrDenied)
		return
	}
	if request.URL.Query().Get("confirmed") != "true" {
		feedback(writer, http.StatusOK, "cancelled", nil)
		return
	}
	if err := server.repository.DeleteOrder(request.PathValue("id"), principal(request)); err != nil {
		server.actionError(writer, err)
		return
	}
	http.Redirect(writer, request, "/orders", http.StatusSeeOther)
}

func (server *Server) transitionOrder(writer http.ResponseWriter, request *http.Request) {
	action := domain.Action(request.PathValue("action"))
	confirmed := request.URL.Query().Get("confirmed") == "true"
	if action == domain.ActionApprove && !confirmed {
		if !domain.Allowed(principal(request), domain.OrderApprove) {
			server.actionError(writer, domain.ErrDenied)
			return
		}
		feedback(writer, http.StatusOK, "cancelled", nil)
		return
	}
	order, err := server.repository.TransitionOrder(request.PathValue("id"), action, principal(request), confirmed)
	if err != nil {
		server.actionError(writer, err)
		return
	}
	target := "/orders/" + order.ID
	if request.URL.Query().Get("from") == "list" {
		target = "/orders"
	}
	http.Redirect(writer, request, target, http.StatusSeeOther)
}

func (server *Server) stockItems(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.StockItemsView) {
		return
	}
	parameters := request.URL.Query()
	pageNumber := requestedPage(parameters.Get("page"))
	page, err := server.repository.StockItems(store.StockQuery{
		Search: parameters.Get("search"), ProductID: parameters.Get("product"), Page: pageNumber,
	})
	if err != nil {
		feedback(writer, http.StatusInternalServerError, "failure", nil)
		return
	}
	var body strings.Builder
	body.WriteString("<h1>Stock items</h1><div data-fields=\"location product onHand reserved\"></div>")
	body.WriteString("<div data-actions=\"view edit\"></div><form><input name=\"search\"><select name=\"product\"></select></form>")
	if len(page.Items) == 0 {
		body.WriteString("<p role=status>empty</p>")
	}
	for _, item := range page.Items {
		id := html.EscapeString(item.ID)
		fmt.Fprintf(&body, "<article data-id=\"%s\"><a data-action=\"view\" href=\"/stock-items/%s\">%s</a><a data-action=\"edit\" href=\"/stock-items/%s/edit\">edit</a></article>", id, id, html.EscapeString(item.Location), id)
	}
	if page.HasMore {
		parameters.Set("page", strconv.Itoa(pageNumber+1))
		fmt.Fprintf(&body, "<a rel=\"next\" href=\"/stock-items?%s\">next</a>", html.EscapeString(parameters.Encode()))
	}
	respond(writer, http.StatusOK, body.String())
}

func (server *Server) stockItemDetail(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.StockItemDetail) {
		return
	}
	item, err := server.repository.StockItem(request.PathValue("id"))
	if err != nil {
		server.readError(writer, err)
		return
	}
	body := fmt.Sprintf(
		"<h1>%s</h1><div data-fields=\"location product onHand reserved\"></div><div data-actions=\"edit\"></div><a data-action=\"edit\" href=\"/stock-items/%s/edit\">edit</a>",
		html.EscapeString(item.Location), html.EscapeString(item.ID),
	)
	respond(writer, http.StatusOK, body)
}

func (server *Server) stockItemEdit(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.StockItemEdit) {
		return
	}
	item, err := server.repository.StockItem(request.PathValue("id"))
	if err != nil {
		server.readError(writer, err)
		return
	}
	respond(writer, http.StatusOK, stockForm(item, server.nextToken()))
}

func (server *Server) updateStockItem(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.StockItemEdit) {
		return
	}
	if err := request.ParseForm(); err != nil {
		feedback(writer, http.StatusUnprocessableEntity, "invalid", nil)
		return
	}
	onHand, onHandOK := requiredInteger(request.Form.Get("onHand"))
	reserved, reservedOK := requiredInteger(request.Form.Get("reserved"))
	preserved := map[string]string{
		"location": request.Form.Get("location"), "product": request.Form.Get("product"),
		"onHand": request.Form.Get("onHand"), "reserved": request.Form.Get("reserved"),
	}
	if strings.TrimSpace(preserved["location"]) == "" || !onHandOK || !reservedOK {
		feedback(writer, http.StatusUnprocessableEntity, "invalid", preserved)
		return
	}
	item, err := server.repository.UpdateStockItem(request.PathValue("id"), store.StockInput{
		ProductID: preserved["product"], Location: preserved["location"], OnHand: onHand, Reserved: reserved,
	}, request.Form.Get("_submission"))
	if err != nil {
		server.mutationError(writer, err, preserved)
		return
	}
	http.Redirect(writer, request, "/stock-items/"+item.ID, http.StatusSeeOther)
}

func (server *Server) reservations(writer http.ResponseWriter, request *http.Request) {
	if !authorize(writer, request, domain.ReservationsView) {
		return
	}
	items, err := server.repository.StockReservations()
	if err != nil {
		feedback(writer, http.StatusInternalServerError, "failure", nil)
		return
	}
	var body strings.Builder
	body.WriteString("<h1>Reservations</h1><div data-fields=\"code stock plan requestedReserved status\"></div><div data-actions=\"commit\"></div>")
	if len(items) == 0 {
		body.WriteString("<p role=status>empty</p>")
	}
	for _, reservation := range items {
		id := html.EscapeString(reservation.ID)
		fmt.Fprintf(&body, "<article data-id=\"%s\">%s<form data-action=\"commit\" data-confirm=\"required\" action=\"/reservations/%s/actions/commit?confirmed=true\"></form></article>",
			id, html.EscapeString(reservation.Code), id)
	}
	respond(writer, http.StatusOK, body.String())
}

func (server *Server) commitReservation(writer http.ResponseWriter, request *http.Request) {
	principal := principal(request)
	if !domain.Allowed(principal, domain.ReservationCommit) {
		server.actionError(writer, domain.ErrDenied)
		return
	}
	confirmed := request.URL.Query().Get("confirmed") == "true"
	if !confirmed {
		feedback(writer, http.StatusOK, "cancelled", nil)
		return
	}
	reservation, _, err := server.repository.CommitStockReservation(request.PathValue("id"), principal, true)
	if err != nil {
		server.actionError(writer, err)
		return
	}
	http.Redirect(writer, request, "/reservations#"+reservation.ID, http.StatusSeeOther)
}

func orderForm(id, number, customer, token string) string {
	return fmt.Sprintf(
		"<form data-id=\"%s\"><input name=\"number\" value=\"%s\"><select name=\"customer\"><option value=\"%s\"></option></select><input name=\"_submission\" value=\"%s\"></form>",
		html.EscapeString(id), html.EscapeString(number), html.EscapeString(customer), html.EscapeString(token),
	)
}

func stockForm(item domain.StockItem, token string) string {
	return fmt.Sprintf(
		"<form data-id=\"%s\"><input name=\"location\" value=\"%s\"><select name=\"product\"><option value=\"%s\"></option></select><input name=\"onHand\" value=\"%d\"><input name=\"reserved\" value=\"%d\"><input name=\"_submission\" value=\"%s\"></form>",
		html.EscapeString(item.ID), html.EscapeString(item.Location), html.EscapeString(item.ProductID), item.OnHand, item.Reserved, html.EscapeString(token),
	)
}

func (server *Server) nextToken() string {
	return fmt.Sprintf("submission-%d", server.tokens.Add(1))
}

func actionPath(action string) string {
	if action == "delete" {
		return "delete"
	}
	return "actions/" + action
}

func confirmationAttribute(action string) string {
	if action == "delete" || action == string(domain.ActionApprove) {
		return " data-confirm=\"required\""
	}
	return ""
}

func actionQuery(action, from string) string {
	values := make([]string, 0, 2)
	if from != "" {
		values = append(values, "from="+from)
	}
	if action == "delete" || action == string(domain.ActionApprove) {
		values = append(values, "confirmed=true")
	}
	if len(values) == 0 {
		return ""
	}
	return "?" + strings.Join(values, "&")
}

func (server *Server) mutationError(writer http.ResponseWriter, err error, preserved map[string]string) {
	if errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrInvariant) {
		feedback(writer, http.StatusUnprocessableEntity, "invalid", preserved)
		return
	}
	feedback(writer, http.StatusInternalServerError, "failure", preserved)
}

func (server *Server) readError(writer http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		feedback(writer, http.StatusNotFound, "empty", nil)
		return
	}
	feedback(writer, http.StatusInternalServerError, "failure", nil)
}

func (server *Server) actionError(writer http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrDenied) {
		http.Error(writer, "denied", http.StatusForbidden)
		return
	}
	if errors.Is(err, domain.ErrInvalidTransition) || errors.Is(err, domain.ErrConfirmationNeeded) {
		http.Error(writer, "invalid", http.StatusConflict)
		return
	}
	if errors.Is(err, domain.ErrInvariant) || errors.Is(err, domain.ErrInvalid) {
		feedback(writer, http.StatusUnprocessableEntity, "invalid", nil)
		return
	}
	if errors.Is(err, domain.ErrTargetUnavailable) || errors.Is(err, domain.ErrValueUnavailable) {
		feedback(writer, http.StatusInternalServerError, "failure", nil)
		return
	}
	server.readError(writer, err)
}

func principal(request *http.Request) domain.Principal {
	var roles []domain.Role
	for _, value := range strings.Split(request.Header.Get("X-Roles"), ",") {
		switch strings.TrimSpace(value) {
		case string(domain.RoleAdmin):
			roles = append(roles, domain.RoleAdmin)
		case string(domain.RoleStaff):
			roles = append(roles, domain.RoleStaff)
		}
	}
	return domain.NewPrincipal(roles...)
}

func authorize(writer http.ResponseWriter, request *http.Request, capability domain.Capability) bool {
	if domain.Allowed(principal(request), capability) {
		return true
	}
	http.Error(writer, "denied", http.StatusForbidden)
	return false
}

func requiredInteger(value string) (int, bool) {
	if strings.TrimSpace(value) == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil && parsed >= 0
}

func requestedPage(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 1
	}
	return parsed
}

func feedback(writer http.ResponseWriter, status int, state string, preserved map[string]string) {
	var body strings.Builder
	fmt.Fprintf(&body, "<p role=status>%s</p>", html.EscapeString(state))
	for name, value := range preserved {
		fmt.Fprintf(&body, "<input name=\"%s\" value=\"%s\">", html.EscapeString(name), html.EscapeString(value))
	}
	respond(writer, status, body.String())
}

func respond(writer http.ResponseWriter, status int, body string) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = fmt.Fprint(writer, "<!doctype html><html><body>"+body+"</body></html>")
}
