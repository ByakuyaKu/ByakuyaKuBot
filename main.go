package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	tgbotapi "github.com/Syfaro/telegram-bot-api"

	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/api/params"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
)

var (
	btnCancel    = tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancelPost")
	btnPost      = tgbotapi.NewInlineKeyboardButtonData("Post", "post")
	btnSetOffset = tgbotapi.NewInlineKeyboardButtonData("Set offset", "setOffset")
	btnPostWall  = tgbotapi.NewInlineKeyboardButtonData("Find and post wallpost from vk", "postWall")
	btnFindPost  = tgbotapi.NewInlineKeyboardButtonData("Find post from vk", "find")

	menu = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			btnPostWall,
		),
	)

	wallPostMenu = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			btnPost,
			btnSetOffset,
			btnCancel),
	)
)

func main() {
	//#region VK API
	err := godotenv.Load() //Load file .env
	if err != nil {
		log.Print(err)
	}
	vkToken := os.Getenv("vkToken")
	vkGroupToken := os.Getenv("vkGroupToken")
	vkGroupID, _ := strconv.Atoi(os.Getenv("vkGroupID"))
	vk := api.NewVK(vkToken)
	vkG := api.NewVK(vkGroupToken)

	//#endregion

	//#region TG API
	tgToken := os.Getenv("tgToken")
	tgChannelID, _ := strconv.ParseInt(os.Getenv("tgChannelID"), 10, 64)
	botChatID, _ := strconv.ParseInt(os.Getenv("botChatID"), 10, 64)

	bot, err := tgbotapi.NewBotAPI(tgToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	go StartLongPollHandling(bot, botChatID, tgChannelID, vkG, vkGroupID)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	waitingWallPostOffset := false
	currentPost := tgbotapi.NewMediaGroup(botChatID, nil)

	for update := range updates {
		if update.Message != nil {
			if update.Message.IsCommand() {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
				switch update.Message.Command() {
				case "menu":
					waitingWallPostOffset = false
					msg.Text = "Please choose action:"
					msg.ReplyMarkup = menu
				default:
					msg.Text = "I don't know that command."
				}
				bot.Send(msg)
			} else if waitingWallPostOffset {
				responseMarkupWPM := tgbotapi.NewEditMessageReplyMarkup(botChatID, update.Message.MessageID, wallPostMenu)
				offset, err := strconv.Atoi(update.Message.Text)
				switch err {
				case nil:
					waitingWallPostOffset = false

					res := GetWallPost(vkGroupID, offset, "owner", vk)

					media := make([]interface{}, len(res.Items[0].Attachments))
					for j := 0; j < len(res.Items[0].Attachments); j++ {
						media[j] = tgbotapi.NewInputMediaPhoto(res.Items[0].Attachments[j].Photo.MaxSize().URL)
					}
					currentPost.ChatID = botChatID
					currentPost.InputMedia = media
					bot.Send(currentPost)
					responseMsg := tgbotapi.NewMessage(botChatID, "Press \"post\" for post this in tg channel.")
					responseMsg.ReplyMarkup = responseMarkupWPM.ReplyMarkup
					bot.Send(responseMsg)
				default:
					log.Print(err)
					responseMsg := tgbotapi.NewMessage(botChatID, "Error! Offset for wallpost must be integer value. Please input correct value.")
					responseMsg.ReplyMarkup = responseMarkupWPM.ReplyMarkup
					bot.Send(responseMsg)
				}

			}
		}
		if update.CallbackQuery != nil {
			bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data))
			responseMarkupWPM := tgbotapi.NewEditMessageReplyMarkup(botChatID, update.CallbackQuery.Message.MessageID, wallPostMenu)
			responseMarkupM := tgbotapi.NewEditMessageReplyMarkup(botChatID, update.CallbackQuery.Message.MessageID, menu)
			switch update.CallbackQuery.Data {
			case "post":
				if currentPost.InputMedia == nil {
					responseMsg := tgbotapi.NewEditMessageText(botChatID, update.CallbackQuery.Message.MessageID, "Error! No post! Set offset first. Please choose action:")
					responseMsg.BaseEdit.ReplyMarkup = responseMarkupWPM.ReplyMarkup
					bot.Send(responseMsg)
				}
				currentPost.ChatID = tgChannelID
				bot.Send(currentPost)
				currentPost.InputMedia = nil

			case "cancelPost":
				waitingWallPostOffset = false
				currentPost.InputMedia = nil
				responseMsg := tgbotapi.NewEditMessageText(botChatID, update.CallbackQuery.Message.MessageID, "Bot menu:")
				responseMsg.BaseEdit.ReplyMarkup = responseMarkupM.ReplyMarkup
				bot.Send(responseMsg)
			case "setOffset":
				waitingWallPostOffset = true
				currentPost.InputMedia = nil
				responseMsg := tgbotapi.NewEditMessageText(botChatID, update.CallbackQuery.Message.MessageID, "Please input offset for wallpost, only integer value")
				responseMsg.BaseEdit.ReplyMarkup = responseMarkupWPM.ReplyMarkup
				bot.Send(responseMsg)
			case "postWall":
				responseMsg := tgbotapi.NewEditMessageText(botChatID, update.CallbackQuery.Message.MessageID, "Please choose action:")
				responseMsg.BaseEdit.ReplyMarkup = responseMarkupWPM.ReplyMarkup
				bot.Send(responseMsg)
			default:
				bot.Send(tgbotapi.NewMessage(botChatID, "Error!"))
			}
		}
	}
	//#endregion
}

//GetWallPost returns post from vk, search by offset
func GetWallPost(vkGroupID int, offset int, filter string, vk *api.VK) api.WallGetResponse {
	w := params.NewWallGetBuilder()
	w.Count(1)
	w.Filter(filter)
	w.Offset(offset)
	w.OwnerID(vkGroupID)

	res, err := vk.WallGet(w.Params)
	if err != nil {
		log.Fatal(err)
	}

	return res
}

//PostFromVkToTG sending a lot of posts from vk.wallget api result to tg chat, maximum 100 posts
func PostFromVkToTG(vkGroupID int, count int, filter string, offset int, vk *api.VK, bot *tgbotapi.BotAPI, tgChannelID int64) {
	if count > 100 {
		return
	}
	w := params.NewWallGetBuilder()
	w.OwnerID(vkGroupID)
	w.Count(count)
	w.Filter(filter)
	w.Offset(offset)

	res, err := vk.WallGet(w.Params)
	if err != nil {
		log.Fatal(err)
		return
	}

	for i := len(res.Items) - 1; i >= 0; i-- {
		media := make([]interface{}, len(res.Items[i].Attachments))

		for j := 0; j < len(res.Items[i].Attachments); j++ {
			media[j] = tgbotapi.NewInputMediaPhoto(res.Items[i].Attachments[j].Photo.MaxSize().URL)
		}
		post := tgbotapi.NewMediaGroup(tgChannelID, media)
		bot.Send(post)
		timer1 := time.NewTimer(time.Second * 20)
		<-timer1.C
		log.Print("Timer 1 expired")
	}
}

//SendWallPostVkToTG sending media posts from vk.logpoll api result to tg chat
func SendWallPostVkToTG(obj events.WallPostNewObject, bot *tgbotapi.BotAPI, tgChannelID int64) {
	media := make([]interface{}, len(obj.Attachments))

	for j := 0; j < len(obj.Attachments); j++ {
		media[j] = tgbotapi.NewInputMediaPhoto(obj.Attachments[j].Photo.MaxSize().URL)
	}

	post := tgbotapi.NewMediaGroup(tgChannelID, media)
	bot.Send(post)
}

//StartLongPollHandling starting vk longopll
func StartLongPollHandling(bot *tgbotapi.BotAPI, botChatID int64, tgChannelID int64, vkG *api.VK, vkGroupID int) {
	// Init longpoll
	lp, err := longpoll.NewLongPoll(vkG, vkGroupID*(-1))
	if err != nil {
		log.Fatal(err)
	}

	lp.WallPostNew(func(ctx context.Context, obj events.WallPostNewObject) {
		SendWallPostVkToTG(obj, bot, tgChannelID)
	})

	lp.MessageNew(func(ctx context.Context, obj events.MessageNewObject) {
		if obj.Message.Text != "" {
			bot.Send(tgbotapi.NewMessage(botChatID, "New message: "+obj.Message.Text))
			return
		}
		bot.Send(tgbotapi.NewMessage(botChatID, "New message!"))
	})

	if err := lp.Run(); err != nil {
		log.Fatal(err)
	}
}
