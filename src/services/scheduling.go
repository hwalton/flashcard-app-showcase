package services

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/abstract-tutoring/models"
	"github.com/abstract-tutoring/utils"
)

func LookupNext(currentStatus int, buttonPressed string) (int, int) {
	switch currentStatus {
	case 0:
		switch buttonPressed {
		case "1":
			return 1, 2 * 60
		case "2":
			return 1, 5 * 60
		case "3":
			return 1, 10 * 60
		case "4":
			return 4, 20 * 3600
		}
	case 1:
		switch buttonPressed {
		case "1":
			return 1, 2 * 60
		case "2":
			return 2, 5 * 60
		case "3":
			return 3, 10 * 60
		case "4":
			return 4, 20 * 3600
		}
	case 2:
		switch buttonPressed {
		case "1":
			return 1, 2 * 60
		case "2":
			return 3, 5 * 60
		case "3":
			return 4, 20 * 3600
		case "4":
			return 4, 20 * 3600
		}
	case 3:
		switch buttonPressed {
		case "1":
			return 1, 2 * 60
		case "2":
			return 3, 5 * 60
		case "3":
			return 4, (1*24 - 4) * 3600
		case "4":
			return 4, (2*24 - 4) * 3600
		}
	case 4:
		switch buttonPressed {
		case "1":
			return 1, 2 * 60
		case "2":
			return 3, 5 * 60
		case "3":
			return 4, (2*24 - 4) * 3600
		case "4":
			return 5, (3*24 - 4) * 3600
		}
	case 5:
		switch buttonPressed {
		case "1":
			return 1, 2 * 60
		case "2":
			return 3, 5 * 60
		case "3":
			return 5, (5*24 - 4) * 3600
		case "4":
			return 6, (14*24 - 4) * 3600
		}
	case 6:
		switch buttonPressed {
		case "1":
			return 1, 2 * 60
		case "2":
			return 3, 5 * 60
		case "3":
			return 6, (31*24 - 4) * 3600
		case "4":
			return 6, (62*24 - 4) * 3600
		}
	}
	// fallback if no valid transition
	return -1, -1
}

func PickNextCard(
	reviewAheadOffset int64,
	cards []models.StudentCard,
	numNewCardsToday int,
	maxNewCardsToday string,
	allCards map[string]models.Flashcard,
	allowedTags map[string]bool,
) (string, bool, error) {

	now := time.Now().Unix() + reviewAheadOffset

	var newCardsDue, inProgressCards, inProgressCardsDue, reviewCardsDue []models.StudentCard

	for _, card := range cards {
		fullCard, ok := allCards[card.CardID]
		if !ok || !CardMatchesTags(fullCard, allowedTags) {
			continue
		}

		if card.Status == 1 || card.Status == 2 || card.Status == 3 {
			inProgressCards = append(inProgressCards, card)
		}
		if card.Due > now {
			continue
		}
		switch card.Status {
		case 0:
			newCardsDue = append(newCardsDue, card)
		case 1, 2, 3:
			inProgressCardsDue = append(inProgressCardsDue, card)
		case 4, 5, 6:
			reviewCardsDue = append(reviewCardsDue, card)
		}
	}

	var options [][]models.StudentCard
	categoryFlags := []bool{false, false, false}

	i, err := strconv.Atoi(maxNewCardsToday)
	if err != nil {
		return "", false, errors.New("invalid maxNewCardsToday value")
	}

	if len(reviewCardsDue) > 0 {
		options = append(options, reviewCardsDue)
		categoryFlags[0] = true
	}
	if len(inProgressCardsDue) > 0 {
		options = append(options, inProgressCardsDue)
		categoryFlags[1] = true
	}
	if numNewCardsToday < i && len(inProgressCardsDue) < 5 && len(newCardsDue) > 0 {
		options = append(options, newCardsDue)
		categoryFlags[2] = true
	}

	if len(options) == 0 {
		if len(inProgressCards) > 0 {
			selectedCategory := inProgressCards
			selectedIsNew := false
			soonest, err := getSoonestCard(selectedCategory)
			if err != nil {
				return "", false, err
			}
			return soonest.CardID, selectedIsNew, nil
		}
		return "", false, errors.New("no cards available")
	}

	selectedIndex := rand.Intn(len(options))
	selectedCategory := options[selectedIndex]
	selectedIsNew := categoryFlags[2] && (selectedIndex == len(options)-1)

	soonest, err := getSoonestCard(selectedCategory)
	if err != nil {
		return "", false, err
	}

	return soonest.CardID, selectedIsNew, nil
}

func getSoonestCard(cards []models.StudentCard) (models.StudentCard, error) {
	if len(cards) == 0 {
		return models.StudentCard{}, errors.New("no cards available")
	}

	soonest := cards[0]
	for _, c := range cards {
		if c.Due < soonest.Due {
			soonest = c
		}
	}

	return soonest, nil
}

func FormatDueTime(seconds int, nextStatus int) string {
	absDelta := seconds
	if absDelta < 0 {
		absDelta = -absDelta
	}
	var timeStr string
	switch {
	case absDelta < 90:
		timeStr = fmt.Sprintf("%d sec", absDelta)
	case absDelta < 90*60:
		timeStr = fmt.Sprintf("%d min", int(math.Round(float64(absDelta)/60)))
	case absDelta < 36*3600:
		timeStr = fmt.Sprintf("%d hr", int(math.Round(float64(absDelta)/3600)))
	default:
		timeStr = fmt.Sprintf("%d d", int(math.Round(float64(absDelta)/86400)))
	}
	if nextStatus >= 0 && nextStatus <= 3 {
		timeStr = "<" + timeStr
	}
	return timeStr
}

func MapStatusToLabel(status int) string {
	switch status {
	case 0:
		return "New"
	case 1, 2, 3:
		return "InProgress"
	case 4, 5, 6:
		return "Review"
	default:
		return ""
	}
}

func GetCurrentStreak(streakStartTime, streakEndTime int64) (int, string) {
	endDay := utils.UnixToUKTime(streakEndTime).Format("2006-01-02")

	nowUnix := time.Now().UTC().Unix()
	yesterdayUnix := nowUnix - 24*3600
	yesterdayDay := utils.UnixToUKTime(yesterdayUnix).Format("2006-01-02")

	if endDay < yesterdayDay {
		return 0, "💀"
	}

	startTime := utils.UnixToUKTime(streakStartTime)
	endTime := utils.UnixToUKTime(streakEndTime)
	days := int(endTime.Sub(startTime).Hours()/24) + 1
	if days < 1 {
		days = 1
	}

	var emoji string
	switch {
	case days <= 5:
		emoji = "🔥"
	case days <= 10:
		emoji = "💪"
	case days <= 20:
		emoji = "🥇"
	case days <= 49:
		emoji = "🏆"
	case days <= 99:
		emoji = "💎"
	default:
		emoji = "💯"
	}

	return days, emoji
}
