package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDenied                = errors.New("access denied")
	ErrNotFound              = errors.New("record not found")
	ErrUnavailable           = errors.New("repository unavailable")
	ErrInvalid               = errors.New("invalid input")
	ErrConflict              = errors.New("unique value already exists")
	ErrInvalidTransition     = errors.New("invalid state transition")
	ErrConfirmationNeeded    = errors.New("confirmation required")
	ErrInvariant             = errors.New("stock availability invariant violated")
	ErrTargetUnavailable     = errors.New("change target unavailable")
	ErrValueUnavailable      = errors.New("change value unavailable")
	ErrNumericRepresentation = errors.New("exact numeric result is not representable")
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleStaff Role = "staff"
)

type Principal struct {
	Roles map[Role]bool
}

func NewPrincipal(roles ...Role) Principal {
	result := Principal{Roles: map[Role]bool{}}
	for _, role := range roles {
		result.Roles[role] = true
	}
	return result
}

func (principal Principal) Has(role Role) bool {
	return principal.Roles[role]
}

type Customer struct {
	ID    string
	Name  string
	Email string
}

func (customer Customer) Label() string { return customer.Name }

type Product struct {
	ID    string
	SKU   string
	Name  string
	Price string
}

func (product Product) Label() string { return product.Name }

type StockItem struct {
	ID        string
	ProductID string
	Location  string
	OnHand    int
	Reserved  int
	Version   int
}

func (item StockItem) Label() string { return item.Location }

func ValidateStock(item StockItem) error {
	if strings.TrimSpace(item.Location) == "" || item.OnHand < 0 || item.Reserved < 0 {
		return ErrInvalid
	}
	if item.Reserved > item.OnHand {
		return ErrInvariant
	}
	return nil
}

type OrderStatus string

const (
	OrderDraft     OrderStatus = "Draft"
	OrderSubmitted OrderStatus = "Submitted"
	OrderApproved  OrderStatus = "Approved"
	OrderRejected  OrderStatus = "Rejected"
	OrderShipped   OrderStatus = "Shipped"
)

type Order struct {
	ID         string
	Number     string
	CustomerID string
	LineIDs    []string
	Status     OrderStatus
	Version    int
}

func (order Order) Label() string { return order.Number }

type OrderLine struct {
	ID        string
	OrderID   string
	ProductID string
	Quantity  int
}

type ReservationStatus string

const (
	ReservationPending   ReservationStatus = "Pending"
	ReservationCommitted ReservationStatus = "Committed"
)

type StockReservation struct {
	ID                string
	Code              string
	StockID           string
	PlanID            string
	RequestedReserved int
	Status            ReservationStatus
	Version           int
}

func (reservation StockReservation) Label() string { return reservation.Code }

type ReservationPlan struct {
	ID               string
	Code             string
	ApprovedReserved int
}

func (plan ReservationPlan) Label() string { return plan.Code }

func ValidateOrder(order Order) error {
	if strings.TrimSpace(order.Number) == "" {
		return ErrInvalid
	}
	return nil
}

type Action string

const (
	ActionSubmit  Action = "submit"
	ActionApprove Action = "approve"
	ActionReject  Action = "reject"
	ActionShip    Action = "ship"
)

func Transition(status OrderStatus, action Action, confirmed bool) (OrderStatus, error) {
	switch action {
	case ActionSubmit:
		if status == OrderDraft {
			return OrderSubmitted, nil
		}
	case ActionApprove:
		if !confirmed {
			return status, ErrConfirmationNeeded
		}
		if status == OrderSubmitted {
			return OrderApproved, nil
		}
	case ActionReject:
		if status == OrderSubmitted {
			return OrderRejected, nil
		}
	case ActionShip:
		if status == OrderApproved {
			return OrderShipped, nil
		}
	default:
		return status, fmt.Errorf("%w: unknown action %s", ErrInvalidTransition, action)
	}
	return status, ErrInvalidTransition
}
