package main

import (
	"log"
	"net/http"

	"example.com/forma-orders-target/internal/domain"
	"example.com/forma-orders-target/internal/store"
	"example.com/forma-orders-target/internal/web"
)

func main() {
	repository := store.New()
	customer := repository.PutCustomer(domain.Customer{Name: "Aster Labs", Email: "orders@aster.example"})
	product := repository.PutProduct(domain.Product{SKU: "WIDGET-1", Name: "Widget", Price: "12.50"})
	_, _ = repository.PutStockItem(domain.StockItem{ProductID: product.ID, Location: "Tokyo", OnHand: 10, Reserved: 2})
	_, _ = repository.CreateOrder(store.OrderInput{Number: "ORD-100", CustomerID: customer.ID}, "seed")
	log.Fatal(http.ListenAndServe(":8080", web.New(repository)))
}
