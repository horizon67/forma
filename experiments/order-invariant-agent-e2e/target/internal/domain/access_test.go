package domain

import "testing"

func TestOrdersAccess(t *testing.T) {
	assertAccess(t, OrdersView, true, true, true, false)
}

func TestOrderCreateAccess(t *testing.T) {
	assertAccess(t, OrderCreate, false, true, true, false)
}

func TestOrderDetailAccess(t *testing.T) {
	assertAccess(t, OrderDetail, true, true, true, false)
}

func TestOrderEditAccess(t *testing.T) {
	assertAccess(t, OrderEdit, false, true, true, false)
}

func TestOrderDeleteAccess(t *testing.T) {
	assertAccess(t, OrderDelete, true, true, true, false)
}

func TestOrderSubmitAccess(t *testing.T) {
	assertAccess(t, OrderSubmit, true, true, true, false)
}

func TestOrderApproveAccess(t *testing.T) {
	assertAccess(t, OrderApprove, true, false, true, false)
}

func TestOrderRejectAccess(t *testing.T) {
	assertAccess(t, OrderReject, true, false, true, false)
}

func TestOrderShipAccess(t *testing.T) {
	assertAccess(t, OrderShip, false, true, true, false)
}

func TestStockItemsAccess(t *testing.T) {
	assertAccess(t, StockItemsView, true, true, true, false)
}

func TestStockItemDetailAccess(t *testing.T) {
	assertAccess(t, StockItemDetail, true, true, true, false)
}

func TestStockItemEditAccess(t *testing.T) {
	assertAccess(t, StockItemEdit, true, false, true, false)
}

func assertAccess(t *testing.T, capability Capability, admin, staff, both, anonymous bool) {
	t.Helper()
	cases := []struct {
		name      string
		principal Principal
		want      bool
	}{
		{name: "admin", principal: NewPrincipal(RoleAdmin), want: admin},
		{name: "staff", principal: NewPrincipal(RoleStaff), want: staff},
		{name: "admin+staff", principal: NewPrincipal(RoleAdmin, RoleStaff), want: both},
		{name: "anonymous", principal: NewPrincipal(), want: anonymous},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := Allowed(test.principal, capability); got != test.want {
				t.Fatalf("Allowed(%s) = %t, want %t", capability, got, test.want)
			}
		})
	}
}
