package main

type demoBook struct {
	ID          string
	Title       string
	Description string
	Amount      string
	Currency    string
}

func demoCatalog() []demoBook {
	return []demoBook{
		{ID: "audio-book-001", Title: "Modern Business Audio Book", Description: "Digital audio book purchase", Amount: "640.00", Currency: "ETB"},
		{ID: "audio-book-002", Title: "Leadership Field Notes", Description: "Digital audio book purchase", Amount: "580.00", Currency: "ETB"},
		{ID: "audio-book-003", Title: "Practical Finance Basics", Description: "Digital audio book purchase", Amount: "720.00", Currency: "ETB"},
		{ID: "audio-book-004", Title: "Startup Operations Guide", Description: "Digital audio book purchase", Amount: "690.00", Currency: "ETB"},
		{ID: "audio-book-005", Title: "Customer Service Playbook", Description: "Digital audio book purchase", Amount: "510.00", Currency: "ETB"},
		{ID: "audio-book-006", Title: "Digital Commerce Lessons", Description: "Digital audio book purchase", Amount: "760.00", Currency: "ETB"},
		{ID: "audio-book-007", Title: "Project Delivery Habits", Description: "Digital audio book purchase", Amount: "550.00", Currency: "ETB"},
		{ID: "audio-book-008", Title: "Retail Growth Stories", Description: "Digital audio book purchase", Amount: "615.00", Currency: "ETB"},
		{ID: "audio-book-009", Title: "Resilient Teams", Description: "Digital audio book purchase", Amount: "675.00", Currency: "ETB"},
		{ID: "audio-book-010", Title: "Merchant Payments 101", Description: "Digital audio book purchase", Amount: "705.00", Currency: "ETB"},
	}
}

func findBook(id string) (demoBook, bool) {
	for _, book := range demoCatalog() {
		if book.ID == id {
			return book, true
		}
	}
	return demoBook{}, false
}

func orderFromBook(book demoBook, customerName, merchantReference string) demoOrder {
	return demoOrder{
		MerchantReference: merchantReference,
		ItemID:            book.ID,
		ItemTitle:         book.Title,
		Amount:            book.Amount,
		Currency:          book.Currency,
		CustomerName:      customerName,
		CustomerCode:      merchantReference,
		CustomerPhone:     "",
		Description:       book.Description,
	}
}
