package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"marketmind-ai/internal/models"
)

type CompanyHandler struct {
	DB *sql.DB
}

type CompanyRequest struct {
	Name     string  `json:"name" binding:"required"`
	Symbol   string  `json:"symbol" binding:"required"`
	Exchange string  `json:"exchange" binding:"required"`
	Country  string  `json:"country" binding:"required"`
	Sector   *string `json:"sector"`
	Industry *string `json:"industry"`
	Currency *string `json:"currency"`
	Website  *string `json:"website"`
}

func normalize(req *CompanyRequest) {
	req.Name = strings.TrimSpace(req.Name)
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	req.Exchange = strings.ToUpper(strings.TrimSpace(req.Exchange))
	req.Country = strings.TrimSpace(req.Country)

	if req.Sector != nil {
		*req.Sector = strings.TrimSpace(*req.Sector)
	}
	if req.Industry != nil {
		*req.Industry = strings.TrimSpace(*req.Industry)
	}
	if req.Currency != nil {
		*req.Currency = strings.ToUpper(strings.TrimSpace(*req.Currency))
	}
	if req.Website != nil {
		*req.Website = strings.TrimSpace(*req.Website)
	}
}

func valid(req CompanyRequest) bool {
	return req.Name != "" &&
		req.Symbol != "" &&
		req.Exchange != "" &&
		req.Country != "" &&
		len(req.Name) <= 255 &&
		len(req.Symbol) <= 50 &&
		len(req.Exchange) <= 30 &&
		len(req.Country) <= 100
}

func scanCompany(row interface {
	Scan(...any) error
}) (models.Company, error) {
	var company models.Company

	err := row.Scan(
		&company.ID,
		&company.Name,
		&company.Symbol,
		&company.Exchange,
		&company.Country,
		&company.Sector,
		&company.Industry,
		&company.Currency,
		&company.Website,
		&company.CreatedAt,
		&company.UpdatedAt,
	)

	return company, err
}

const companyColumns = `
	id, name, symbol, exchange, country, sector,
	industry, currency, website, created_at, updated_at
`

func databaseError(c *gin.Context, err error) {
	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		c.JSON(http.StatusConflict, gin.H{"error": "Company symbol already exists on this exchange"})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": "Database operation failed"})
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)

	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return 0, false
	}

	return id, true
}

func (h *CompanyHandler) Create(c *gin.Context) {
	var req CompanyRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company data"})
		return
	}

	normalize(&req)

	if !valid(req) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Required fields are invalid"})
		return
	}

	company, err := scanCompany(h.DB.QueryRow(`
		INSERT INTO companies
		(name, symbol, exchange, country, sector, industry, currency, website)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING `+companyColumns,
		req.Name, req.Symbol, req.Exchange, req.Country,
		req.Sector, req.Industry, req.Currency, req.Website,
	))

	if err != nil {
		databaseError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": company})
}

func (h *CompanyHandler) List(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	search := strings.TrimSpace(c.Query("search"))
	exchange := strings.ToUpper(strings.TrimSpace(c.Query("exchange")))
	country := strings.TrimSpace(c.Query("country"))

	rows, err := h.DB.Query(`
		SELECT `+companyColumns+`
		FROM companies
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%' OR symbol ILIKE '%' || $1 || '%')
		AND ($2 = '' OR exchange = $2)
		AND ($3 = '' OR country ILIKE $3)
		ORDER BY name
		LIMIT $4 OFFSET $5
	`, search, exchange, country, limit, offset)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	companies := make([]models.Company, 0)

	for rows.Next() {
		company, err := scanCompany(rows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read company"})
			return
		}
		companies = append(companies, company)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database iteration failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   companies,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *CompanyHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	company, err := scanCompany(h.DB.QueryRow(
		`SELECT `+companyColumns+` FROM companies WHERE id=$1`, id,
	))

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": company})
}

func (h *CompanyHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var req CompanyRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company data"})
		return
	}

	normalize(&req)

	if !valid(req) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Required fields are invalid"})
		return
	}

	company, err := scanCompany(h.DB.QueryRow(`
		UPDATE companies SET
		name=$1, symbol=$2, exchange=$3, country=$4,
		sector=$5, industry=$6, currency=$7, website=$8,
		updated_at=NOW()
		WHERE id=$9
		RETURNING `+companyColumns,
		req.Name, req.Symbol, req.Exchange, req.Country,
		req.Sector, req.Industry, req.Currency, req.Website, id,
	))

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		databaseError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": company})
}

func (h *CompanyHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	result, err := h.DB.Exec(
		`DELETE FROM companies WHERE id=$1`, id,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database delete failed"})
		return
	}

	affected, _ := result.RowsAffected()

	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	c.Status(http.StatusNoContent)
}
