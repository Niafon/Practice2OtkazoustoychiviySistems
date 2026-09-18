package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"flatbook/web"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type application struct {
	db         *sql.DB
	instanceID string
}

type apartment struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	City        string `json:"city"`
	Address     string `json:"address"`
	Description string `json:"description"`
	PriceNight  int    `json:"price_night"`
	Guests      int    `json:"guests"`
	Rooms       int    `json:"rooms"`
	Rating      string `json:"rating"`
	Accent      string `json:"accent"`
}

type bookingRequest struct {
	ApartmentID int64  `json:"apartment_id"`
	GuestName   string `json:"guest_name"`
	Email       string `json:"email"`
	CheckIn     string `json:"check_in"`
	CheckOut    string `json:"check_out"`
	Guests      int    `json:"guests"`
}

type bookingResponse struct {
	ID           int64  `json:"id"`
	ApartmentID  int64  `json:"apartment_id"`
	Confirmation string `json:"confirmation"`
	BackendNode  string `json:"backend_node"`
}

func main() {
	port := env("PORT", "8080")
	instanceID := env("INSTANCE_ID", hostnameOr("backend-local"))
	databaseURL := env("DATABASE_URL", "postgres://flatbook:flatbook@localhost:5432/flatbook?sslmode=disable")

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("database is unavailable: %v", err)
	}

	app := &application{db: db, instanceID: instanceID}
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(web.Files, ".")
	if err != nil {
		log.Fatalf("prepare static files: %v", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /healthz", app.health)
	mux.HandleFunc("GET /api/node", app.nodeInfo)
	mux.HandleFunc("GET /api/apartments", app.listApartments)
	mux.HandleFunc("POST /api/bookings", app.createBooking)
	mux.HandleFunc("GET /api/bookings/{id}", app.getBooking)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           app.withCommonHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("FlatBook node=%s listening on http://0.0.0.0:%s", instanceID, port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-done
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func (app *application) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Node", app.instanceID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func (app *application) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := "ok"
	code := http.StatusOK
	if err := app.db.PingContext(ctx); err != nil {
		status = "database_unavailable"
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status": status,
		"node":   app.instanceID,
	})
}

func (app *application) nodeInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"backend_node": app.instanceID,
		"hostname":     hostnameOr("unknown"),
		"time":         time.Now().UTC().Format(time.RFC3339),
	})
}

func (app *application) listApartments(w http.ResponseWriter, r *http.Request) {
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	guests := 1
	if raw := r.URL.Query().Get("guests"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 20 {
			errorJSON(w, http.StatusBadRequest, "guests must be from 1 to 20")
			return
		}
		guests = n
	}

	checkIn, checkOut := strings.TrimSpace(r.URL.Query().Get("check_in")), strings.TrimSpace(r.URL.Query().Get("check_out"))
	if (checkIn == "") != (checkOut == "") {
		errorJSON(w, http.StatusBadRequest, "check_in and check_out must be provided together")
		return
	}
	if checkIn != "" {
		if err := validateDateRange(checkIn, checkOut); err != nil {
			errorJSON(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	query := `
        SELECT a.id, a.title, a.city, a.address, a.description,
               a.price_night, a.guests, a.rooms, a.rating::text, a.accent
        FROM apartments a
        WHERE ($1 = '' OR lower(a.city) LIKE '%' || lower($1) || '%')
          AND a.guests >= $2
          AND (
              $3 = '' OR NOT EXISTS (
                  SELECT 1
                  FROM bookings b
                  WHERE b.apartment_id = a.id
                    AND daterange(b.check_in, b.check_out, '[)') && daterange($3::date, $4::date, '[)')
              )
          )
        ORDER BY a.rating DESC, a.id
    `

	rows, err := app.db.QueryContext(r.Context(), query, city, guests, checkIn, checkOut)
	if err != nil {
		log.Printf("list apartments: %v", err)
		errorJSON(w, http.StatusServiceUnavailable, "storage is temporarily unavailable")
		return
	}
	defer rows.Close()

	items := make([]apartment, 0)
	for rows.Next() {
		var a apartment
		if err := rows.Scan(&a.ID, &a.Title, &a.City, &a.Address, &a.Description, &a.PriceNight, &a.Guests, &a.Rooms, &a.Rating, &a.Accent); err != nil {
			log.Printf("scan apartment: %v", err)
			errorJSON(w, http.StatusInternalServerError, "cannot read apartments")
			return
		}
		items = append(items, a)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"backend_node": app.instanceID,
	})
}

func (app *application) createBooking(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req bookingRequest
	if err := dec.Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.GuestName = strings.TrimSpace(req.GuestName)
	req.Email = strings.TrimSpace(req.Email)
	if req.ApartmentID < 1 || req.GuestName == "" || !strings.Contains(req.Email, "@") || req.Guests < 1 {
		errorJSON(w, http.StatusBadRequest, "fill apartment, name, email and guests")
		return
	}
	if err := validateDateRange(req.CheckIn, req.CheckOut); err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	tx, err := app.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		errorJSON(w, http.StatusServiceUnavailable, "storage is temporarily unavailable")
		return
	}
	defer tx.Rollback()

	var capacity int
	if err := tx.QueryRowContext(r.Context(), `SELECT guests FROM apartments WHERE id=$1`, req.ApartmentID).Scan(&capacity); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "apartment not found")
			return
		}
		errorJSON(w, http.StatusServiceUnavailable, "storage is temporarily unavailable")
		return
	}
	if req.Guests > capacity {
		errorJSON(w, http.StatusBadRequest, "too many guests for this apartment")
		return
	}

	var id int64
	var confirmation string
	err = tx.QueryRowContext(r.Context(), `
        INSERT INTO bookings (apartment_id, guest_name, email, check_in, check_out, guests)
        VALUES ($1, $2, $3, $4::date, $5::date, $6)
        RETURNING id, confirmation
    `, req.ApartmentID, req.GuestName, req.Email, req.CheckIn, req.CheckOut, req.Guests).Scan(&id, &confirmation)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			errorJSON(w, http.StatusConflict, "these dates have already been booked")
			return
		}
		log.Printf("create booking: %v", err)
		errorJSON(w, http.StatusServiceUnavailable, "storage is temporarily unavailable")
		return
	}

	if err := tx.Commit(); err != nil {
		errorJSON(w, http.StatusServiceUnavailable, "storage is temporarily unavailable")
		return
	}

	writeJSON(w, http.StatusCreated, bookingResponse{
		ID:           id,
		ApartmentID:  req.ApartmentID,
		Confirmation: confirmation,
		BackendNode:  app.instanceID,
	})
}

func (app *application) getBooking(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		errorJSON(w, http.StatusBadRequest, "invalid booking id")
		return
	}

	var result struct {
		ID           int64  `json:"id"`
		Confirmation string `json:"confirmation"`
		Apartment    string `json:"apartment"`
		City         string `json:"city"`
		CheckIn      string `json:"check_in"`
		CheckOut     string `json:"check_out"`
		Guests       int    `json:"guests"`
	}
	err = app.db.QueryRowContext(r.Context(), `
        SELECT b.id, b.confirmation, a.title, a.city,
               b.check_in::text, b.check_out::text, b.guests
        FROM bookings b
        JOIN apartments a ON a.id = b.apartment_id
        WHERE b.id = $1
    `, id).Scan(&result.ID, &result.Confirmation, &result.Apartment, &result.City, &result.CheckIn, &result.CheckOut, &result.Guests)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "booking not found")
			return
		}
		errorJSON(w, http.StatusServiceUnavailable, "storage is temporarily unavailable")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"booking":      result,
		"backend_node": app.instanceID,
	})
}

func validateDateRange(in, out string) error {
	checkIn, err := time.Parse("2006-01-02", in)
	if err != nil {
		return fmt.Errorf("check_in must be YYYY-MM-DD")
	}
	checkOut, err := time.Parse("2006-01-02", out)
	if err != nil {
		return fmt.Errorf("check_out must be YYYY-MM-DD")
	}
	if !checkOut.After(checkIn) {
		return fmt.Errorf("check_out must be later than check_in")
	}
	if checkOut.Sub(checkIn) > 60*24*time.Hour {
		return fmt.Errorf("booking cannot be longer than 60 days")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func errorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func hostnameOr(fallback string) string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return fallback
	}
	return h
}
