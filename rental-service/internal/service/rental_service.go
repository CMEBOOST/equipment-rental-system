package service

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/rental-service/internal/client"
	"github.com/equipment-rental-system/rental-service/internal/model"
	"github.com/equipment-rental-system/rental-service/internal/repository"
)

var (
	ErrInvalidDates      = errors.New("due_date must be later than start_date")
	ErrInvalidDateFormat = errors.New("date must use YYYY-MM-DD")
	ErrInvalidReturnDate = errors.New("return_date must not be before start_date")
	ErrInvalidState      = errors.New("rental is not in a valid state for this action")
)

type RentalService struct {
	repo     *repository.RentalRepository
	products *client.ProductClient
	users    *client.UserClient
}

func NewRentalService(repo *repository.RentalRepository, products *client.ProductClient, users *client.UserClient) *RentalService {
	return &RentalService{repo: repo, products: products, users: users}
}

func (s *RentalService) Create(userID, productID uuid.UUID, startRaw, dueRaw, callerToken string, pending bool) (*model.Rental, error) {
	if err := s.users.VerifyCaller(callerToken); err != nil {
		return nil, err
	}
	start, due, err := parseDates(startRaw, dueRaw)
	if err != nil {
		return nil, err
	}
	product, err := s.products.Get(productID)
	if err != nil {
		return nil, err
	}
	if product.Status != "available" {
		return nil, client.ErrProductUnavailable
	}
	rental := &model.Rental{ID: uuid.New(), UserID: userID, ProductID: productID, StartDate: start, DueDate: due, TotalPrice: roundCurrency(product.PricePerDay * float64(daysBetween(start, due))), Status: model.StatusActive}
	if pending {
		rental.Status = model.StatusPending
		if err := s.repo.Create(rental); err != nil {
			return nil, err
		}
		return rental, nil
	}
	// Product owns availability. Reserve it first; compensate if our local write fails.
	if err := s.products.SetStatus(productID, "rented"); err != nil {
		return nil, err
	}
	if err := s.repo.Create(rental); err != nil {
		_ = s.products.SetStatus(productID, "available")
		return nil, err
	}
	return rental, nil
}

func (s *RentalService) Approve(id uuid.UUID, callerToken string) (*model.Rental, error) {
	if err := s.users.VerifyCaller(callerToken); err != nil {
		return nil, err
	}
	rental, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if rental.Status != model.StatusPending {
		return nil, ErrInvalidState
	}
	product, err := s.products.Get(rental.ProductID)
	if err != nil {
		return nil, err
	}
	if product.Status != "available" {
		return nil, client.ErrProductUnavailable
	}
	if err := s.products.SetStatus(rental.ProductID, "rented"); err != nil {
		return nil, err
	}
	rental.Status = model.StatusActive
	if err := s.repo.Update(rental); err != nil {
		_ = s.products.SetStatus(rental.ProductID, "available")
		return nil, err
	}
	return rental, nil
}

func (s *RentalService) Return(id uuid.UUID, returnRaw, callerToken string) (*model.Rental, error) {
	if err := s.users.VerifyCaller(callerToken); err != nil {
		return nil, err
	}
	rental, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if rental.Status != model.StatusActive {
		return nil, ErrInvalidState
	}
	returned, err := time.Parse("2006-01-02", returnRaw)
	if err != nil {
		return nil, ErrInvalidDateFormat
	}
	if returned.Before(rental.StartDate) {
		return nil, ErrInvalidReturnDate
	}
	if err := s.products.SetStatus(rental.ProductID, "available"); err != nil {
		return nil, err
	}
	rental.Status, rental.ReturnDate = model.StatusReturned, &returned
	if err := s.repo.Update(rental); err != nil {
		_ = s.products.SetStatus(rental.ProductID, "rented")
		return nil, err
	}
	return rental, nil
}

func (s *RentalService) Get(id uuid.UUID) (*model.Rental, error) { return s.repo.Get(id) }
func (s *RentalService) List(filter repository.RentalFilter) ([]model.Rental, int64, error) {
	return s.repo.List(filter)
}

func parseDates(startRaw, dueRaw string) (time.Time, time.Time, error) {
	start, err := time.Parse("2006-01-02", startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidDateFormat
	}
	due, err := time.Parse("2006-01-02", dueRaw)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidDateFormat
	}
	if !due.After(start) {
		return time.Time{}, time.Time{}, ErrInvalidDates
	}
	return start, due, nil
}

func daysBetween(start, due time.Time) int { return int(due.Sub(start).Hours() / 24) }

func roundCurrency(amount float64) float64 { return math.Round(amount*100) / 100 }

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, client.ErrProductNotFound)
}
