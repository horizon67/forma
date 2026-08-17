package domain

type Plan string

const (
	PlanFree       Plan = "Free"
	PlanPro        Plan = "Pro"
	PlanEnterprise Plan = "Enterprise"
)

var Plans = []Plan{PlanFree, PlanPro, PlanEnterprise}

func ValidPlan(value Plan) bool {
	for _, plan := range Plans {
		if value == plan {
			return true
		}
	}
	return false
}

type Status string

const (
	StatusPending   Status = "Pending"
	StatusConfirmed Status = "Confirmed"
	StatusActive    Status = "Active"
	StatusSuspended Status = "Suspended"
)

var Statuses = []Status{StatusPending, StatusConfirmed, StatusActive, StatusSuspended}

func ValidStatus(value Status) bool {
	for _, status := range Statuses {
		if value == status {
			return true
		}
	}
	return false
}

type Team struct {
	ID   string
	Name string
}

type User struct {
	ID       string
	Name     string
	Nickname string
	Email    string
	TeamID   string
	Plan     Plan
	Status   Status
}
