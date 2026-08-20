package domain

type Capability string

const (
	OrdersView      Capability = "orders.view"
	OrderCreate     Capability = "order.create"
	OrderDetail     Capability = "order.detail"
	OrderEdit       Capability = "order.edit"
	OrderDelete     Capability = "order.delete"
	OrderSubmit     Capability = "order.submit"
	OrderApprove    Capability = "order.approve"
	OrderReject     Capability = "order.reject"
	OrderShip       Capability = "order.ship"
	StockItemsView  Capability = "stock-items.view"
	StockItemDetail Capability = "stock-item.detail"
	StockItemEdit   Capability = "stock-item.edit"
)

func Allowed(principal Principal, capability Capability) bool {
	switch capability {
	case OrdersView, OrderDetail, OrderDelete, OrderSubmit, StockItemsView, StockItemDetail:
		return principal.Has(RoleAdmin) || principal.Has(RoleStaff)
	case OrderCreate, OrderEdit, OrderShip:
		return principal.Has(RoleStaff)
	case OrderApprove, OrderReject, StockItemEdit:
		return principal.Has(RoleAdmin)
	default:
		return false
	}
}
