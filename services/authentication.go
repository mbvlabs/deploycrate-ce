package services

import (
	"context"

	"errors"

	"fmt"

	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailNotVerified   = errors.New("email not verified")
)

type LoginData struct {
	Email    string
	Password string
}

func (i Identity) AuthenticateUser(
	ctx context.Context,
	data LoginData,
) (models.UserEntity, error) {
	b := validation.NewBuilder()
	b.Required("email", data.Email)
	b.Required("password", data.Password)
	if !b.Errors().Empty() {
		return models.UserEntity{}, b.Errors()
	}

	user, err := models.User.FindByEmail(ctx, i.db.Executor(), data.Email)

	if err != nil {

		if errors.Is(err, models.ErrNotFound) {
			return models.UserEntity{}, ErrInvalidCredentials
		}

		return models.UserEntity{}, fmt.Errorf("find user by email: %w", err)

	}

	return i.authenticatePassword(ctx, user, data.Password)
}

func (i Identity) VerifyUserPassword(
	ctx context.Context,
	userID uuid.UUID,
	password string,
) error {
	if userID == uuid.Nil || password == "" {
		return ErrInvalidCredentials
	}

	user, err := models.User.Find(ctx, i.db.Executor(), userID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("find user: %w", err)
	}

	_, err = i.authenticatePassword(ctx, user, password)
	return err
}

func (i Identity) authenticatePassword(
	ctx context.Context,
	user models.UserEntity,
	password string,
) (models.UserEntity, error) {
	validPassword, needsRehash, err := verifyPasswordWithPeppers(
		user,
		password,
		i.pepper,
		i.previousPeppers,
	)

	if err != nil {

		return models.UserEntity{}, fmt.Errorf("validate password: %w", err)

	}

	if !validPassword {

		return models.UserEntity{}, ErrInvalidCredentials
	}

	if needsRehash {
		hashedPassword, err := models.HashPassword(password, i.pepper)
		if err != nil {
			return models.UserEntity{}, fmt.Errorf("rehash password with current pepper: %w", err)
		}

		user, err = models.User.Update(ctx, i.db.Executor(), models.UpdateUserData{
			ID:               user.ID,
			Email:            user.Email,
			EmailValidatedAt: user.EmailValidatedAt,
			Password:         []byte(hashedPassword),
			IsAdmin:          user.IsAdmin,
		})
		if err != nil {
			return models.UserEntity{}, fmt.Errorf("persist password rehash: %w", err)
		}
	}

	if !user.HasValidatedEmail() {
		return models.UserEntity{}, ErrEmailNotVerified
	}

	return user, nil
}

func verifyPasswordWithPeppers(
	user models.UserEntity,
	providedPassword string,
	currentPepper string,
	previousPeppers []string,
) (valid bool, needsRehash bool, err error) {
	valid, err = user.ValidPassword(providedPassword, currentPepper)
	if err != nil || valid {
		return valid, false, err
	}

	for _, previousPepper := range previousPeppers {
		valid, err = user.ValidPassword(providedPassword, previousPepper)
		if err != nil {
			return false, false, err
		}
		if valid {
			return true, true, nil
		}
	}

	return false, false, nil
}
