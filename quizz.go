package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type QuestionTrend struct {
	Question string   `json:"question"`
	Choices  []string `json:"choices"`
	Answer   string   `json:"answer"`
}

type QuizResult struct {
	UserID string
	Score  int
}

const (
	TopArtistsQuestionType = iota
	GenreQuestionType
	RegionalTrendsQuestionType
)

// génère une question sur la popularité des artistes
func generateTopArtistsQuestion(db *mongo.Database) (QuestionTrend, error) {
	artistsCollection := db.Collection("artists")
	//sélectionne 4 artistes aléatoires dans la collection artists
	pipeline := mongo.Pipeline{
		{{Key: "$sample", Value: bson.D{{Key: "size", Value: 4}}}},
	}
	cursor, err := artistsCollection.Aggregate(context.Background(), pipeline)
	if err != nil {
		log.Printf("Erreur lors de l'agrégation des artistes: %v", err)
		return QuestionTrend{}, fmt.Errorf("erreur lors de l'agrégation des artistes: %w", err)
	}
	defer cursor.Close(context.TODO())

	var artists []Artist
	if err = cursor.All(context.TODO(), &artists); err != nil {
		log.Printf("Erreur lors de la récupération des artistes: %v", err)
		return QuestionTrend{}, fmt.Errorf("erreur lors de la récupération des artistes: %w", err)
	}
	if len(artists) == 0 {
		log.Println("Aucun artiste trouvé")
		return QuestionTrend{}, fmt.Errorf("aucun artiste trouvé")
	}

	//création de la question
	question := QuestionTrend{
		Choices:  make([]string, 0, 4),
		Question: "Quel est l'artiste le plus streamé?",
	}

	//détermine quel artiste est le plus populaire parmi les 4 et le met comme réponse
	var mostPopular Artist
	for _, artist := range artists {
		question.Choices = append(question.Choices, artist.Name)
		if mostPopular.Name == "" || artist.Popularity > mostPopular.Popularity {
			mostPopular = artist
		}
	}

	return question, nil
}

// génère une question sur le genre le plus représenté parmi les artistes
func generateGenreQuestion(db *mongo.Database) (QuestionTrend, error) {

	genres := []string{"j-pop", "rock", "jazz", "blues", "classical", "rap",
		"r&b", "pop", "hip hop", "french hip hop", "k-pop"}

	//choisis aléatoirement un genre principal et un genre secondaire
	rand.Shuffle(len(genres), func(i, j int) { genres[i], genres[j] = genres[j], genres[i] })
	mainGenre := genres[0]
	secondaryGenre := genres[1] //en s'assurant qu'ils soient différent

	// sélectionne 3 artistes correspondant au genre principal et 1 artiste avec un autre genere
	artistsCollection := db.Collection("artists")
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "genre", Value: mainGenre}}}},
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 3}}}},
		bson.D{{Key: "$unionWith", Value: bson.D{
			{Key: "coll", Value: "artists"},
			{Key: "pipeline", Value: mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "genre", Value: secondaryGenre}}}},
				bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 1}}}},
			}},
		}}},
	}

	cursor, err := artistsCollection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de l'agrégation des artistes: %w", err)
	}
	defer cursor.Close(context.TODO())

	var artists []Artist
	if err = cursor.All(context.TODO(), &artists); err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de la récupération des artistes: %w", err)
	}
	if len(artists) == 0 {
		return QuestionTrend{}, fmt.Errorf("aucun artiste trouvé pour générer une question de genre")
	}

	//création de la question
	question := QuestionTrend{
		Question: fmt.Sprintf("Quel est le genre musical le plus représenté parmi ces artistes : %s,  %s,  %s,  %s",
			artists[0].Name,
			artists[1].Name,
			artists[2].Name,
			artists[3].Name),
		Answer:  mainGenre,
		Choices: make([]string, 0, 4),
	}
	//création des choix
	question.Choices = append(question.Choices, mainGenre)
	question.Choices = append(question.Choices, secondaryGenre)
	rand.Shuffle(len(genres), func(i, j int) { genres[i], genres[j] = genres[j], genres[i] })

	for _, genre := range genres {
		if genre != mainGenre && genre != secondaryGenre && len(question.Choices) < 4 {
			question.Choices = append(question.Choices, genre)
		}
	}
	// Mélanger les choix
	rand.Shuffle(len(question.Choices), func(i, j int) { question.Choices[i], question.Choices[j] = question.Choices[j], question.Choices[i] })

	return question, nil
}

// génère une question sur la popularité d'une piste dans un pays
func generateRegionalTrendsQuestion(db *mongo.Database) (QuestionTrend, error) {
	top50Collection := db.Collection("top50")
	//sélectionne 4 playlists aléatoires de la collection 'top50'
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 4}}}},
	}

	cursor, err := top50Collection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de l'agrégation pour une tendance régionale: %w", err)
	}
	defer cursor.Close(context.TODO())

	var countryTracksList []CountryTracks
	if err = cursor.All(context.TODO(), &countryTracksList); err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de la récupération des données de tendance régionale: %w", err)
	}
	if len(countryTracksList) < 4 {
		return QuestionTrend{}, fmt.Errorf("pas assez de données de tendance régionale trouvées")
	}

	questionType := rand.Intn(2) //génère aléatoirement 0 ou 1 pour le type de question
	var question QuestionTrend
	question.Choices = make([]string, 0, 4)

	if questionType == 0 { //question ayant pour choix un pays
		selectedTrackIndex := rand.Intn(len(countryTracksList[0].Tracks))
		selectedTrack := countryTracksList[0].Tracks[selectedTrackIndex]

		question.Question = fmt.Sprintf("Dans quel pays la piste '%s' est-elle la plus populaire?", selectedTrack.Name)
		question.Answer = countryTracksList[0].Country
		//ajoute pour choix les autres nom de pays
		for _, countryTracks := range countryTracksList[1:] {
			question.Choices = append(question.Choices, countryTracks.Country)
		}
		question.Choices = append(question.Choices, question.Answer)

	} else { //question ayant pour choix le nom d'une piste
		selectedCountryTracks := countryTracksList[0]
		question.Question = fmt.Sprintf("Quelle est la piste la plus populaire en %s?", selectedCountryTracks.Country)
		mostPopular := selectedCountryTracks.Tracks[0] //la piste la plus populaire d'un pays est à l'indice 0 de la playlist
		question.Answer = mostPopular.Name
		//ajoute pour choix les autres nom de track de la playlist sélectionné
		for i, track := range selectedCountryTracks.Tracks {
			if i > 0 && len(question.Choices) < 3 {
				question.Choices = append(question.Choices, track.Name)
			}
		}
		question.Choices = append(question.Choices, question.Answer)
	}

	rand.Shuffle(len(question.Choices), func(i, j int) { question.Choices[i], question.Choices[j] = question.Choices[j], question.Choices[i] })

	return question, nil
}

// --------------- Handler gérant les données des quizzs ---------------------

// handler pour générer une question de quiz
func generateQuizQuestionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var question QuestionTrend
	client, err := connectToMongo()
	if err != nil {
		http.Error(w, "Erreur lors de la connexion à MongoDB: "+err.Error(), http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer client.Disconnect(context.TODO())

	db := client.Database("spotifyData")

	validQuestion := false
	for attempts := 0; attempts < 3 && !validQuestion; attempts++ {
		questionType := rand.Intn(3)

		//génère une question en fonction du type
		switch questionType {
		case TopArtistsQuestionType:
			question, err = generateTopArtistsQuestion(db)
		case GenreQuestionType:
			question, err = generateGenreQuestion(db)
		case RegionalTrendsQuestionType:
			question, err = generateRegionalTrendsQuestion(db)
		}

		if err == nil && len(question.Choices) > 0 {
			validQuestion = true
		}
	}

	if !validQuestion {
		http.Error(w, "Impossible de générer une question valide après plusieurs tentatives", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(question)
}

// handler pour finir un quiz et mettre à jour les infos de l'utilisateur
func finishQuizHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	tokenHeader := r.Header.Get("Authorization")
	if tokenHeader == "" {
		http.Error(w, "Authorization header is required", http.StatusUnauthorized)
		return
	}
	splitToken := strings.Split(tokenHeader, "Bearer ")
	if len(splitToken) != 2 {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}
	tokenString := splitToken[1]
	claims := &jwt.StandardClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}

	userID := strings.TrimSpace(claims.Subject)

	if _, ok := token.Claims.(*jwt.StandardClaims); ok && token.Valid {

		//décode les données JSON de la fin du quiz
		var quizResult QuizResult
		err = json.NewDecoder(r.Body).Decode(&quizResult)
		if err != nil {
			http.Error(w, "Erreur lors de la lecture des données JSON", http.StatusBadRequest)
			return
		}

		client, err := connectToMongo()
		if err != nil {
			http.Error(w, "Erreur lors de la connexion à MongoDB", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		defer client.Disconnect(context.Background())

		//mise à jour de l'utilisateur
		collection := client.Database("spotTrendQuizzer").Collection("users")
		filter := bson.M{"userid": userID}
		log.Println("userID", filter)
		// mise à jour du score total, du nombre de parties, et ajouter le score à l'historique
		update := bson.D{
			{Key: "$inc", Value: bson.M{
				"scoretotal":  quizResult.Score,
				"nbdeparties": 1,
			}},
			{Key: "$push", Value: bson.M{
				"scorehistory": bson.M{
					"$each":  []interface{}{fmt.Sprintf("%d", quizResult.Score)},
					"$slice": -5,
				},
			}},
		}

		_, err = collection.UpdateOne(context.Background(), filter, update)
		if err != nil {
			http.Error(w, "Erreur lors de la mise à jour du score de l'utilisateur", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		//mise à jour du score total dans la collection 'classement'
		classementCollection := client.Database("spotTrendQuizzer").Collection("classement")
		classementUpdate := bson.M{
			"$inc": bson.M{"scoreTotal": quizResult.Score},
		}
		_, err = classementCollection.UpdateOne(context.Background(), bson.M{"userId": userID}, classementUpdate)
		if err != nil {
			http.Error(w, "Erreur lors de la mise à jour du score total dans le classement", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		//calcul du nouveau classement de l'utilisateur après la mise à jour du score total
		userRanking, err := getRanking(client, userID)
		if err != nil {
			http.Error(w, "Erreur lors de la récupération du nouveau classement", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		//mettre à jour le classement de l'utilisateur dans la collection 'users'
		_, err = collection.UpdateOne(
			context.Background(),
			bson.M{"userid": userID},
			bson.M{"$set": bson.M{"userRanking": userRanking}},
		)
		if err != nil {
			http.Error(w, "Erreur lors de la mise à jour du classement de l'utilisateur", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Mise à jour réussie")
	}
}

// handler appelé à la fin d'un quizz pour transmettre les infos mis à jours de l'utilisateur
func getQuizResultHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	tokenHeader := r.Header.Get("Authorization")
	splitToken := strings.Split(tokenHeader, "Bearer ")
	if len(splitToken) != 2 {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}
	tokenString := splitToken[1]
	claims := &jwt.StandardClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}
	client, err := connectToMongo()
	if err != nil {
		http.Error(w, "Erreur lors de la connexion à MongoDB", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer client.Disconnect(context.Background())

	//récupération des informations de l'utilisateur de la collection users
	usersCollection := client.Database("spotTrendQuizzer").Collection("users")

	var user User
	err = usersCollection.FindOne(context.Background(), bson.M{"userid": claims.Subject}).Decode(&user)
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des informations de l'utilisateur", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	//récupère le classement de l'utilisateur
	userRanking, err := getRanking(client, user.UserID)
	if err != nil {
		http.Error(w, "Erreur lors de la récupération du classement", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	//création de la réponse
	response := struct {
		ScoreTotal  int           `json:"scoreTotal"`
		UserRanking []UserRanking `json:"userRanking"`
	}{
		ScoreTotal:  user.ScoreTotal,
		UserRanking: userRanking,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
