package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nitrajka/al_ny/pkg/auth"
	"github.com/nitrajka/al_ny/pkg/db"
)

type mockDB struct {
	inPlace map[uint64]*db.DBUser
	lastID  uint64
}

func (q *mockDB) CreateUser(u *db.DBUser) (*db.DBUser, error) {
	u.ID = q.lastID + 1
	q.lastID++
	u.Password, _ = db.HashPassword(u.Password)
	q.inPlace[u.ID] = u

	return u, nil
}

func (q *mockDB) GetUserByID(id uint64) (*db.DBUser, error) {
	if id > q.lastID {
		return &db.DBUser{}, errors.New("sorry, such user does not exist")
	}

	return q.inPlace[id], nil
}

func (q *mockDB) UpdateUser(u *db.UpdateUserBody, userID uint64) (*db.DBUser, error) {
	if userID > q.lastID {
		return &db.DBUser{}, errors.New("sorry, such user does not exist")
	}

	oldUser := q.inPlace[userID]

	newUser := &db.DBUser{
		ID:          oldUser.ID,
		Credentials: db.Credentials{Username: u.Username, Password: oldUser.Password},
		FullName:    u.FullName,
		Address:     u.Address,
		Phone:       u.Phone,
	}

	q.inPlace[newUser.ID] = newUser
	return q.inPlace[newUser.ID], nil
}

func (q *mockDB) UserExistsByCredentials(cred db.Credentials) (*db.DBUser, bool, error) {
	for id := range q.inPlace {
		if q.inPlace[id].Username == cred.Username {
			return q.inPlace[id], true, nil
		}
	}

	return &db.DBUser{}, false, nil
}

func (q *mockDB) ResetPassword(c db.Credentials) error {
	return nil
}

func TestAuthWithPassword(t *testing.T) {
	p1, _ := db.HashPassword("password")
	datab := &mockDB{inPlace: map[uint64]*db.DBUser{
		1: {ID: 1, Credentials: db.Credentials{Username: "email@example.com", Password: p1},
			FullName: "DBUser Novotny", Phone: "09090909", Address: "cool address", SignedUpWithGoogle: false},
		2: {ID: 2, Credentials: db.Credentials{Username: "example@gmail.com", Password: p1}, SignedUpWithGoogle: true},
	}, lastID: 1}

	aut := auth.NewSessionAuth([]byte("secret-key"), "secret-session")

	ma, err := NewApp(datab, aut, nil, aut)
	if err != nil {
		t.Error("could not create App")
	}

	server, err := NewUserServer(ma)
	if err != nil {
		t.Errorf("could not create server: %v", err)
	}

	t.Run("test successful login + logout", func(t *testing.T) {
		request := newPostLoginRequest(t, "email@example.com", "password")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		userWithToken := getUserWithToken(t, response.Body)

		assertStatus(t, response.Code, http.StatusOK)
		assertUser(t, userWithToken.User, *db.DBUserToUser(datab.inPlace[1]))

		request = newPostLogoutRequest(t, userWithToken.Token, userWithToken.User.ID)
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)
		msg := getMessageFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusOK)
		assertMessage(t, msg, "successfully logged out")
	})

	t.Run("test empty login credentials", func(t *testing.T) {
		request := newPostLoginRequest(t, "", "")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		msg := getMessageFromResponse(t, response.Body)
		//token := getTokenFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusBadRequest)
		assertMessage(t, msg, InvalidBodyError(fmt.Errorf("fields must not be empty")))
		//tokenValid()
	})

	t.Run("test user does not exist", func(t *testing.T) {
		request := newPostLoginRequest(t, "emailf@example.com", "password")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		body := getMessageFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusNotFound)
		assertMessage(t, body, NotFoundUserError("email", "emailf@example.com"))
	})

	t.Run("test bad password", func(t *testing.T) {
		request := newPostLoginRequest(t, "email@example.com", "passwordd")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		body := getMessageFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusUnauthorized)
		assertMessage(t, body, IncorrectPasswordError("email@example.com"))
	})

	t.Run("test login with password when previous google registration", func(t *testing.T) {
		request := newPostLoginRequest(t, "example@gmail.com", "password")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		body := getMessageFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusConflict)
		assertMessage(t, body, InvalidLoginType(nil))
	})

	t.Run("test signup successful", func(t *testing.T) {
		newMail := "emailik@example.com"
		request := newPostSignupRequest(t, newMail, "password")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		body := getUserWithToken(t, response.Body)
		expected := &db.SignUpResponse{Token: "", User: db.User{
			ID: 2, Username: newMail, FullName: "", Phone: "", Address: "", SignedUpWithGoogle: false}}

		assertStatus(t, response.Code, http.StatusCreated)
		assertSignupResponseBody(t, body, expected)
	})

	t.Run("test signup username already exists", func(t *testing.T) {
		request := newPostSignupRequest(t, "email@example.com", "password")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		body := getMessageFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusConflict)
		assertMessage(t, body, UserAlreadyExists("email@example.com"))
	})
}

func TestUsersWithPasswordAuth(t *testing.T) {
	p1, _ := db.HashPassword("password")
	datab := &mockDB{inPlace: map[uint64]*db.DBUser{
		1: {ID: 1, Credentials: db.Credentials{Username: "email@example.com", Password: p1},
			FullName: "Petra Novotna", Phone: "09090909", Address: "cool address", SignedUpWithGoogle: false},
		2: {ID: 2, Credentials: db.Credentials{Username: "example@gmail.com", Password: p1}, SignedUpWithGoogle: true},
		3: {ID: 3, Credentials: db.Credentials{Username: "mail@example.com", Password: p1}, SignedUpWithGoogle: false},
	}, lastID: 1}

	aut := auth.NewSessionAuth([]byte("secret-key"), "secret-session-name")

	ma, err := NewApp(datab, aut, nil, aut)
	if err != nil {
		t.Error("could not create App")
	}

	server, err := NewUserServer(ma)
	if err != nil {
		t.Errorf("could not create server: %v", err)
	}

	t.Run("get user successfully", func(t *testing.T) {
		request := newPostLoginRequest(t, "email@example.com", "password")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		userWithToken := getUserWithToken(t, response.Body)

		assertStatus(t, response.Code, http.StatusOK)
		assertUser(t, userWithToken.User, *db.DBUserToUser(datab.inPlace[1]))

		fmt.Println(userWithToken.Token, userWithToken.User.ID)
		request = newGetUserRequest(t, userWithToken.Token, userWithToken.User.ID)
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)
		user := getUserFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusOK)
		assertUser(t, *db.DBUserToUser(datab.inPlace[1]), *user)
	})

	t.Run("update user successfully", func(t *testing.T) {
		request := newPostLoginRequest(t, "email@example.com", "password")
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		userWithToken := getUserWithToken(t, response.Body)

		assertStatus(t, response.Code, http.StatusOK)
		assertUser(t, userWithToken.User, *db.DBUserToUser(datab.inPlace[1]))

		dbuser := datab.inPlace[1]
		user := &db.UpdateUserBody{
			Username: dbuser.Username,
			FullName: "Petra Novakova",
			Phone:    dbuser.Phone,
			Address:  dbuser.Address,
		}

		request = newPutUserRequest(t, user, userWithToken.Token, dbuser.ID)
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)
		newUser := getUserFromResponse(t, response.Body)

		expected := db.DBUserToUser(dbuser)
		expected.FullName = "Petra Novakova"

		assertStatus(t, response.Code, http.StatusOK)
		assertUser(t, *newUser, *expected)
	})

	invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhY2Nlc3NfdXVpZCI6Ijg2ZDEzZDRlLTNiMGMtNDA5ZC05YWEwLTBiZTkxZmZlMTgxYSIsImF1dGhvcmlzZWQiOnRydWUsImV4cCI6MTU5Njk3Mzg5MiwidXNlcl9pZCI6MX0.lyY0Q6qWf2jCU_I-mp4KLummRTJ6J0weYqA-2lUPdPs"

	t.Run("cannot get user without auth", func(t *testing.T) {
		request := newGetUserRequest(t, invalidToken, 1)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		msg := getMessageFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusUnauthorized)
		assertMessage(t, msg, UnauthorizedError(nil))
	})

	t.Run("cannot update user without auth", func(t *testing.T) {
		dbuser := datab.inPlace[1]
		user := &db.UpdateUserBody{
			Username: dbuser.Username,
			FullName: dbuser.FullName,
			Phone:    dbuser.Phone,
			Address:  dbuser.Address,
		}

		user.FullName = "Petra Novakova"
		request := newPutUserRequest(t, user, invalidToken, dbuser.ID)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		msg := getMessageFromResponse(t, response.Body)

		assertStatus(t, response.Code, http.StatusUnauthorized)
		assertMessage(t, msg, UnauthorizedError(nil))
	})
}

//----------------------------create request helpers--------------------------------

func newPostLoginRequest(t *testing.T, username, password string) *http.Request {
	req, err := http.NewRequest(
		http.MethodPost,
		"/login",
		strings.NewReader(`{"username": "`+username+`", "password": "`+password+`"}`))
	if err != nil {
		t.Errorf("something went wrong creating a request: %v", err)
	}
	return req
}

func newPostLogoutRequest(t *testing.T, token string, id uint64) *http.Request {
	req, err := http.NewRequest(http.MethodPost, "/logout", strings.NewReader(fmt.Sprintf(`%d`, id)))
	if err != nil {
		t.Errorf("something went wrong creating a request: %v", err)
		return nil
	}

	req.Header.Add("Authorization", token)

	return req
}

func newPostSignupRequest(t *testing.T, username, password string) *http.Request {
	req, err := http.NewRequest(
		http.MethodPost,
		"/signup",
		strings.NewReader(`{"username": "`+username+`", "password": "`+password+`"}`))
	if err != nil {
		t.Errorf("something went wrong creating a request: %v", err)
	}
	return req
}

func newGetUserRequest(t *testing.T, token string, userID uint64) *http.Request {
	path := "/user/" + fmt.Sprintf(`%d`, userID)
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		t.Errorf("something went wrong creating a request: %v", err)
	}

	req.Header.Add("Authorization", token)
	return req
}

func newPutUserRequest(t *testing.T, u *db.UpdateUserBody, token string, userID uint64) *http.Request {
	path := "/user/" + fmt.Sprintf(`%d`, userID)
	req, err := http.NewRequest(
		http.MethodPut,
		path,
		strings.NewReader(
			fmt.Sprintf(`{"address": "%s", "phone": "%s", "fullname": "%s", "username": "%s" }`, u.Address, u.Phone, u.FullName, u.Username)))
	if err != nil {
		t.Errorf("something went wrong creating a request: %v", err)
	}

	req.Header.Add("Authorization", token)

	return req
}

//------------------------------decode response helpers--------------------------------

func getUserWithToken(t *testing.T, buff *bytes.Buffer) *db.SignUpResponse {
	t.Helper()

	var resp *db.SignUpResponse

	err := json.NewDecoder(buff).Decode(&resp)
	if err != nil {
		t.Errorf("signup response incorrect: %v", err.Error())
		return nil
	}

	return resp
}

func getTokenFromResponse(t *testing.T, resp *bytes.Buffer) string {
	t.Helper()

	var sr *db.SignUpResponse
	err := json.NewDecoder(resp).Decode(&sr)
	if err != nil {
		t.Error("error while decoding token from response")
	}

	return sr.Token
}

func getMessageFromResponse(t *testing.T, resp *bytes.Buffer) (msg string) {
	t.Helper()

	err := json.NewDecoder(resp).Decode(&msg)
	if err != nil {
		t.Errorf("error while decoding response message: %v", err)
	}

	return
}

func getUserFromResponse(t *testing.T, resp *bytes.Buffer) (user *db.User) {
	t.Helper()

	err := json.NewDecoder(resp).Decode(&user)
	if err != nil {
		t.Errorf("error while decoding DBUser in response: %v", err)
	}

	return
}

//------------------------------------assertion helpers----------------------------------------

func assertStatus(t *testing.T, actual, expected int) {
	t.Helper()
	if actual != expected {
		t.Errorf("did not get correct status, actual %d, expected %d", actual, expected)
	}
}

func assertMessage(t *testing.T, actual, expected string) {
	t.Helper()
	if actual != expected {
		t.Errorf("msg not right: actual %s, expected %s", actual, expected)
	}
}

func assertSignupResponseBody(t *testing.T, actual, expected *db.SignUpResponse) {
	t.Helper()

	assertUser(t, actual.User, expected.User)
}

func assertUser(t *testing.T, actual, expected db.User) {
	t.Helper()

	if actual.ID != expected.ID ||
		actual.FullName != expected.FullName ||
		actual.Address != expected.Address ||
		actual.Phone != expected.Phone ||
		actual.Username != expected.Username ||
		actual.SignedUpWithGoogle != expected.SignedUpWithGoogle {
		t.Errorf("user does not match: actual: %v, expected: %v", actual, expected)
	}
}