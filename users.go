package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/golang-jwt/jwt"
)

// Structure pour représenter un utilisateur
type User struct {
	UserID       string        `json:"userID"`
	Pseudo       string        `json:"pseudo"`
	Password     string        `json:"password"`
	ScoreTotal   int           `json:"scoreTotal"`
	NbDeParties  int           `json:"nbDeParties"`
	ScoreHistory []string      `json:"scoreHistory"`
	UserRanking  []UserRanking `json:"UserRanking"`
}

type UserRanking struct {
	UserID string `bson: "userId" json: "userId"`
	Pseudo string `bson:"pseudo" json:"pseudo"`
	Score  int    `bson:"scoreTotal" json:"scoreTotal"`
	Rank   int    `bson:"rank" json:"rank"`
}

// Clé secrète utilisée pour signer le token
var jwtKey = []byte("cleSecret")

// génère un ID utilisateur unique en combinant la date et l'heure actuelles avec une partie aléatoire
func generateUniqueUserID() string {
	//obtient date et heure actuelles
	currentTime := time.Now().UTC()

	timeString := currentTime.Format("20060102T150405Z") //format simplifié sans caractères spéciaux

	//génère une partie aléatoire pour garantir l'unicité de l'ID
	randomPart := rand.Int63()

	uniqueID := fmt.Sprintf("%s-%d", timeString, randomPart)

	return uniqueID
}

func getTopPlayers(client *mongo.Client) ([]UserRanking, error) {
	collection := client.Database("spotTrendQuizzer").Collection("classement")

	// Récupérer tous les utilisateurs triés par score décroissant
	cursor, err := collection.Find(context.Background(), bson.D{}, options.Find().SetSort(bson.D{{Key: "scoreTotal", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	// Parcourir le curseur pour remplir la slice des joueurs
	var players []UserRanking
	if err = cursor.All(context.Background(), &players); err != nil {
		return nil, err
	}

	// Attribuer des rangs aux joueurs en fonction de leur position dans la liste triée
	currentRank := 1
	for i := 0; i < len(players); i++ {
		// Si le score du joueur actuel est différent du score du joueur précédent,
		// ou si c'est le premier joueur, attribuer le rang actuel
		if i == 0 || players[i].Score != players[i-1].Score {
			players[i].Rank = currentRank
			currentRank++
		} else {
			// Si le score est le même, attribuer le même rang que le joueur précédent
			players[i].Rank = players[i-1].Rank
		}
	}

	// Conserver uniquement les meilleurs joueurs jusqu'à la limite de 5 rangs uniques
	var topPlayers []UserRanking
	rankSet := make(map[int]struct{})
	for _, player := range players {
		if len(rankSet) < 5 {
			if _, exists := rankSet[player.Rank]; !exists {
				rankSet[player.Rank] = struct{}{}
				topPlayers = append(topPlayers, player)
			}
		} else {
			break
		}
	}

	return topPlayers, nil
}

// Créer le token JWT pour l'utilisateur
func generateToken(userID string) (string, error) {

	log.Printf("userid recu dans generatetoken %s", userID)
	// Préparer les claims du token
	claims := &jwt.StandardClaims{
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		Issuer:    "spotTrendQuizzer",
		Subject:   userID,
	}

	// Créer un nouveau token JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Signer le token avec ta clé secrète
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// getRanking récupère le classement autour d'un utilisateur donné.
func getRanking(client *mongo.Client, userID string) ([]UserRanking, error) {
	collection := client.Database("spotTrendQuizzer").Collection("classement")

	log.Printf("-----Dans classement utilisateur------------")

	// recupere tous les users depuis collection
	cursor, err := collection.Find(context.Background(), bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var users []UserRanking
	if err = cursor.All(context.Background(), &users); err != nil {
		return nil, err
	}

	// Trier les utilisateurs par score en ordre décroissant
	sort.Slice(users, func(i, j int) bool {
		return users[i].Score > users[j].Score
	})

	// Attribuer des rangs aux utilisateurs en fonction de leur position dans la liste triée
	currentRank := 1
	for i := 0; i < len(users); i++ {
		// Si le score de l'utilisateur actuel est différent du score de l'utilisateur précédent,
		// ou si c'est le premier utilisateur, attribuer le rang actuel
		if i == 0 || users[i].Score != users[i-1].Score {
			users[i].Rank = currentRank
			currentRank++
		} else {
			// Si le score est le même, attribuer le même rang que l'utilisateur précédent
			users[i].Rank = users[i-1].Rank
		}
	}

	// Trouver l'utilisateur actuel
	var currentUser UserRanking
	for _, user := range users {
		if user.UserID == userID {
			currentUser = user
			break
		}
	}

	// Trouver l'utilisateur juste avant et juste après l'utilisateur actuel
	var prevUser, nextUser UserRanking
	for _, user := range users {
		if user.UserID != currentUser.UserID {
			if user.Rank <= currentUser.Rank-1 {
				prevUser = user
			} else if user.Rank >= currentUser.Rank+1 {
				nextUser = user
				break // On arrête dès que l'utilisateur suivant est trouvé
			}
		}
	}

	// Créer le classement autour de l'utilisateur actuel
	var ranking []UserRanking
	if prevUser.Pseudo != "" { // Vérifier si un utilisateur précédent a été trouvé
		ranking = append(ranking, prevUser)
	}
	ranking = append(ranking, currentUser)
	if nextUser.Pseudo != "" { // Vérifier si un utilisateur suivant a été trouvé
		ranking = append(ranking, nextUser)
	}

	return ranking, nil
}

// Handler pour la requête d'inscription (sign up)
func signUpHandler(w http.ResponseWriter, r *http.Request) {
	// Autoriser les en-têtes, méthodes et origines nécessaires pour CORS
	w.Header().Set("Access-Control-Allow-Origin", "*") // À ajuster selon vos besoins de sécurité
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Gérer la pré-vérification CORS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var newUser User

	err := json.NewDecoder(r.Body).Decode(&newUser)
	if err != nil {
		http.Error(w, "Erreur lors de la lecture des données JSON", http.StatusBadRequest)
		return
	}

	// Vérifiez si les champs importants sont présents
	if newUser.Pseudo == "" || newUser.Password == "" {
		http.Error(w, "Pseudonyme et mot de passe requis", http.StatusBadRequest)
		return
	}

	//Connexion à Mongo
	client, err := connectToMongo()
	if err != nil {
		http.Error(w, "Erreur lors de la connexion à MongoDB", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	defer client.Disconnect(context.Background())

	//Vérifie si le pseudonyme est unique
	collection := client.Database("spotTrendQuizzer").Collection("users")
	var existingUser User
	err = collection.FindOne(context.Background(), bson.M{"pseudo": newUser.Pseudo}).Decode(&existingUser)
	if err == nil {
		http.Error(w, "Le pseudonyme est déjà pris", http.StatusBadRequest)
		return
	} else if err != mongo.ErrNoDocuments {
		http.Error(w, "Erreur lors de la recherche de l'utilisateur dans la base de données", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	//Génération de l'ID utilisateur unique
	newUser.UserID = generateUniqueUserID()
	log.Printf("token genere %s", newUser.UserID)

	//Initialise le tableau de l'historique des scores avec une liste vide
	newUser.ScoreHistory = []string{}

	//Initialisation des champs ScoreTotal et NbDeParties
	newUser.ScoreTotal = 0
	newUser.NbDeParties = 0

	//Ajoute le nouvel utilisateur à la base de données
	_, err = collection.InsertOne(context.Background(), newUser)
	if err != nil {
		http.Error(w, "Erreur lors de l'ajout de l'utilisateur à la base de données", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// Insérez maintenant le nouvel utilisateur dans la collection 'classement'
	classementCollection := client.Database("spotTrendQuizzer").Collection("classement")

	// Créez un objet pour représenter la nouvelle entrée dans la collection 'classement'
	classementEntry := bson.M{
		"userId":     newUser.UserID,
		"pseudo":     newUser.Pseudo,
		"scoreTotal": newUser.ScoreTotal, // qui est 0 pour un nouvel utilisateur
	}

	// Insérez l'entrée dans la collection 'classement'
	_, err = classementCollection.InsertOne(context.Background(), classementEntry)
	if err != nil {
		http.Error(w, "Erreur lors de l'ajout de l'utilisateur à la collection de classement", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// Réponse de succès
	//w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Utilisateur enregistré avec succès !"})
}

// Handler pour la requête de connexion (sign in)
func signInHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Gérer la pré-vérification CORS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Décodez les données JSON de la requête de connexion
	var signInInfo struct {
		Pseudo   string `json:"pseudo"`
		Password string `json:"password"`
	}
	err := json.NewDecoder(r.Body).Decode(&signInInfo)
	if err != nil {
		http.Error(w, "Erreur lors de la lecture des données JSON", http.StatusBadRequest)
		return
	}

	// Connectez-vous à MongoDB
	client, err := connectToMongo()
	if err != nil {
		http.Error(w, "Erreur lors de la connexion à MongoDB", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer client.Disconnect(context.Background())

	// Recherchez l'utilisateur dans la base de données
	collection := client.Database("spotTrendQuizzer").Collection("users")
	var user User
	err = collection.FindOne(context.Background(), bson.M{"pseudo": signInInfo.Pseudo, "password": signInInfo.Password}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "Identifiant ou mot de passe incorrect", http.StatusUnauthorized)
		} else {
			http.Error(w, "Erreur lors de la recherche de l'utilisateur dans la base de données", http.StatusInternalServerError)
			log.Println(err)
		}
		return
	}

	if err != nil {
		http.Error(w, "Erreur lors de la récupération des meilleurs joueurs", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// Générer le token JWT
	tokenString, err := generateToken(user.UserID)
	if err != nil {
		http.Error(w, "Erreur lors de la creation de token", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// Préparer la réponse qui inclut à la fois le token et les topPlayers
	response := struct {
		Token string `json:"token"`
	}{
		Token: tokenString,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Handler pour récupérer les 5 premiers top players for homepage
func topPlayersHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("dans topPlayersHandler ")

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Gérer la pré-vérification CORS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Connectez-vous à MongoDB (ou utilisez une connexion existante)
	client, err := connectToMongo() // Remplacez ceci par votre fonction de connexion MongoDB réelle
	if err != nil {
		http.Error(w, "Erreur lors de la connexion à la base de données", http.StatusInternalServerError)
		return
	}
	defer client.Disconnect(context.Background())

	// Obtenez les joueurs du haut du classement
	topPlayers, err := getTopPlayers(client)
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des joueurs", http.StatusInternalServerError)
		return
	}

	// Définissez le content-type de votre réponse
	w.Header().Set("Content-Type", "application/json")

	// Encodez le slice topPlayers en JSON et écrivez la réponse
	err = json.NewEncoder(w).Encode(topPlayers)
	if err != nil {
		http.Error(w, "Erreur lors de l'encodage des joueurs en JSON", http.StatusInternalServerError)
	}
}

// Handler pour récupérer les informations de l'utilisateur
func userInfoHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Check if preflight request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	log.Printf("Received token: %s", r.Header.Get("Authorization"))

	// Extraire le token de l'en-tête Authorization
	tokenHeader := r.Header.Get("Authorization")
	if tokenHeader == "" {
		http.Error(w, "Authorization header is required", http.StatusUnauthorized)
		return
	}

	// Supposer que le token est précédé par "Bearer "
	splitToken := strings.Split(tokenHeader, "Bearer ")
	if len(splitToken) != 2 {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}

	tokenString := splitToken[1]
	claims := &jwt.StandardClaims{}

	// La vérification du token doit être ici.
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})

	// Si il y a une erreur de parse ou le token n'est pas valide.
	if err != nil || !token.Valid {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}

	// Log après le parsing du token.
	log.Printf("UserID from token: '%s'", claims.Subject)

	userID := strings.TrimSpace(claims.Subject) // Enlevez les espaces de début et de fin.

	if _, ok := token.Claims.(*jwt.StandardClaims); ok && token.Valid {
		log.Printf("UserID from token: '%s'", userID)

		client, err := connectToMongo()
		if err != nil {
			http.Error(w, "Error connecting to MongoDB", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		defer client.Disconnect(context.Background())

		collection := client.Database("spotTrendQuizzer").Collection("users")
		var user User
		err = collection.FindOne(context.Background(), bson.M{"userid": userID}).Decode(&user)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, "User not found", http.StatusNotFound)
				return
			} else {
				http.Error(w, "Error searching user in the database", http.StatusInternalServerError)
				log.Println(err)
				return
			}
		}
		// Obtenez le classement de l'utilisateur
		ranking, err := getRanking(client, userID)
		if err != nil {
			http.Error(w, "Error retrieving user ranking", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		// Préparez une structure de réponse combinée qui comprend les informations de l'utilisateur et son classement
		response := struct {
			UserInfo User          `json:"userInfo"`
			Ranking  []UserRanking `json:"ranking"`
		}{
			UserInfo: user,
			Ranking:  ranking,
		}

		// Encodez la réponse combinée et envoyez-la au client
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	} else {
		http.Error(w, "Invalid token claims", http.StatusUnauthorized)
	}
}
