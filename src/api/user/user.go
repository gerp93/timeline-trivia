package apiUser

import (
	"net/http"

	gsApi "github.com/gerp93/gameshell-framework/api"
	gsAuth "github.com/gerp93/gameshell-framework/auth"
	gsDatabase "github.com/gerp93/gameshell-framework/database"
	"github.com/google/uuid"
)

func Create(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	var name string
	var password string
	var passwordConfirm string
	for key, val := range r.Form {
		if key == "name" {
			name = val[0]
		} else if key == "password" {
			password = val[0]
		} else if key == "passwordConfirm" {
			passwordConfirm = val[0]
		}
	}

	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No name found."))
		return
	}

	if password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No password found."))
		return
	}

	if password != passwordConfirm {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Passwords do not match."))
		return
	}

	if gsDatabase.UserNameExists(name) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("User name already exists."))
		return
	}

	err = gsDatabase.CreateUser(name, password, false)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Your request has been submitted. Please wait for an administrator to approve this account."))
}

func CreateAdmin(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	var name string
	var password string
	var passwordConfirm string
	for key, val := range r.Form {
		if key == "name" {
			name = val[0]
		} else if key == "password" {
			password = val[0]
		} else if key == "passwordConfirm" {
			passwordConfirm = val[0]
		}
	}

	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No name found."))
		return
	}

	if password == "" {
		password = "password"
	} else if password != passwordConfirm {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Passwords do not match."))
		return
	}

	if gsDatabase.UserNameExists(name) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("User name already exists."))
		return
	}

	err = gsDatabase.CreateUser(name, password, true)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusCreated)
}

func Login(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	var name string
	var password string
	for key, val := range r.Form {
		if key == "name" {
			name = val[0]
		} else if key == "password" {
			password = val[0]
		}
	}

	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No name found."))
		return
	}

	if password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No password found."))
		return
	}

	allowLogin, err := gsDatabase.AllowUserLoginAttempt(r.RemoteAddr, name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if !allowLogin {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Too many login attempts, please wait an hour to try again."))
		return
	}

	err = gsDatabase.AddUserLoginAttempt(r.RemoteAddr, name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if !gsDatabase.UserNameExists(name) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("User name does not exist."))
		return
	}

	userId, err := gsDatabase.GetUserIdByName(name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	isApproved, err := gsDatabase.GetUserIsApproved(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if !isApproved {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("User account is not yet approved by an administrator."))
		return
	}

	passwordHash, err := gsDatabase.GetUserPasswordHash(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if !gsAuth.PasswordMatchesHash(password, passwordHash) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Provided password is not valid."))
		return
	}

	gsAuth.SetUserId(w, userId)

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func Logout(w http.ResponseWriter, _ *http.Request) {
	gsAuth.RemoveUserId(w)
	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func SetName(w http.ResponseWriter, r *http.Request) {
	userIdString := r.PathValue("userId")
	userId, err := uuid.Parse(userIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id from path."))
		return
	}

	if !isCurrentUser(r, userId) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err = r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	var name string
	for key, val := range r.Form {
		if key == "name" {
			name = val[0]
		}
	}

	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No name found."))
		return
	}

	if gsDatabase.UserNameExists(name) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("User name already exists."))
		return
	}

	err = gsDatabase.SetUserName(userId, name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func SetPassword(w http.ResponseWriter, r *http.Request) {
	userIdString := r.PathValue("userId")
	userId, err := uuid.Parse(userIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id from path."))
		return
	}

	if !isCurrentUser(r, userId) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err = r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	var currentPassword string
	var newPassword string
	var newPasswordConfirm string
	for key, val := range r.Form {
		if key == "currentPassword" {
			currentPassword = val[0]
		} else if key == "newPassword" {
			newPassword = val[0]
		} else if key == "newPasswordConfirm" {
			newPasswordConfirm = val[0]
		}
	}

	if currentPassword == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No current password found."))
		return
	}

	passwordHash, err := gsDatabase.GetUserPasswordHash(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if !gsAuth.PasswordMatchesHash(currentPassword, passwordHash) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Provided current password is not valid."))
		return
	}

	if newPassword == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No new password found."))
		return
	}

	if newPassword != newPasswordConfirm {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("New passwords do not match."))
		return
	}

	err = gsDatabase.SetUserPassword(userId, newPassword)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func ResetPassword(w http.ResponseWriter, r *http.Request) {
	userIdString := r.PathValue("userId")
	userId, err := uuid.Parse(userIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id from path."))
		return
	}

	if !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err = gsDatabase.SetUserPassword(userId, "password")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<span class='bi bi-check-square'></span>"))
}

func Approve(w http.ResponseWriter, r *http.Request) {
	userIdString := r.PathValue("userId")
	userId, err := uuid.Parse(userIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id from path."))
		return
	}

	if !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err = gsDatabase.ApproveUser(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<span class='bi bi-check-square'></span>"))
}

func SetColorTheme(w http.ResponseWriter, r *http.Request) {
	userIdString := r.PathValue("userId")
	userId, err := uuid.Parse(userIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id from path."))
		return
	}

	if !isCurrentUser(r, userId) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err = r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	var colorTheme string
	for key, val := range r.Form {
		if key == "colorTheme" {
			colorTheme = val[0]
		}
	}

	if colorTheme == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("No color theme found."))
		return
	}

	err = gsDatabase.SetUserColorTheme(userId, colorTheme)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func SetIsAdmin(w http.ResponseWriter, r *http.Request) {
	userIdString := r.PathValue("userId")
	userId, err := uuid.Parse(userIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id from path."))
		return
	}

	if !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err = r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to parse form."))
		return
	}

	var isAdmin bool
	for key, val := range r.Form {
		if key == "isAdmin" {
			isAdmin = val[0] == "1"
		}
	}

	err = gsDatabase.SetUserIsAdmin(userId, isAdmin)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func Delete(w http.ResponseWriter, r *http.Request) {
	userIdString := r.PathValue("userId")
	userId, err := uuid.Parse(userIdString)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Failed to get user id from path."))
		return
	}

	isCurrentUser := isCurrentUser(r, userId)
	if !isCurrentUser && !isAdmin(r) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("User does not have access."))
		return
	}

	err = gsDatabase.DeleteUser(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if isCurrentUser {
		gsAuth.RemoveUserId(w)
	}

	w.Header().Add("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func isCurrentUser(r *http.Request, checkId uuid.UUID) bool {
	userId := gsApi.GetUserId(r)
	return userId == checkId
}

func isAdmin(r *http.Request) bool {
	userId := gsApi.GetUserId(r)
	if userId == uuid.Nil {
		return false
	}

	isAdmin, _ := gsDatabase.GetUserIsAdmin(userId)
	return isAdmin
}
