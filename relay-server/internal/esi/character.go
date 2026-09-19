package esi

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"
)

type CharacterIdentity struct {
	Name           string  `json:"name"`
	CorporationID  int64   `json:"corporation_id"`
	AllianceID     int64   `json:"alliance_id"`
	SecurityStatus float64 `json:"security_status"`
}
type CharacterOnline struct {
	LastLogin  time.Time `json:"last_login"`
	LastLogout time.Time `json:"last_logout"`
	Logins     int       `json:"logins"`
	Online     bool      `json:"online"`
}
type CharacterLocation struct {
	SolarSystemID int64 `json:"solar_system_id"`
	StationID     int64 `json:"station_id"`
	StructureID   int64 `json:"structure_id"`
}
type CharacterShip struct {
	ShipItemID int64  `json:"ship_item_id"`
	ShipName   string `json:"ship_name"`
	ShipTypeID int64  `json:"ship_type_id"`
}
type CharacterSkills struct {
	Skills        []Skill `json:"skills"`
	TotalSP       int64   `json:"total_sp"`
	UnallocatedSP int64   `json:"unallocated_sp"`
}
type Skill struct {
	ActiveSkillLevel   int   `json:"active_skill_level"`
	SkillID            int64 `json:"skill_id"`
	SkillpointsInSkill int64 `json:"skillpoints_in_skill"`
	TrainedSkillLevel  int   `json:"trained_skill_level"`
}
type SkillQueueEntry struct {
	FinishDate      time.Time `json:"finish_date"`
	FinishedLevel   int       `json:"finished_level"`
	LevelEndSP      int64     `json:"level_end_sp"`
	LevelStartSP    int64     `json:"level_start_sp"`
	QueuePosition   int       `json:"queue_position"`
	SkillID         int64     `json:"skill_id"`
	StartDate       time.Time `json:"start_date"`
	TrainingStartSP int64     `json:"training_start_sp"`
}
type WalletJournalEntry struct {
	Amount        float64   `json:"amount"`
	Balance       float64   `json:"balance"`
	ContextID     int64     `json:"context_id"`
	ContextIDType string    `json:"context_id_type"`
	Date          time.Time `json:"date"`
	Description   string    `json:"description"`
	FirstPartyID  int64     `json:"first_party_id"`
	ID            int64     `json:"id"`
	Reason        string    `json:"reason"`
	RefType       string    `json:"ref_type"`
	SecondPartyID int64     `json:"second_party_id"`
	Tax           float64   `json:"tax"`
	TaxReceiverID int64     `json:"tax_receiver_id"`
}
type Asset struct {
	IsBlueprintCopy bool   `json:"is_blueprint_copy"`
	IsSingleton     bool   `json:"is_singleton"`
	ItemID          int64  `json:"item_id"`
	LocationFlag    string `json:"location_flag"`
	LocationID      int64  `json:"location_id"`
	LocationType    string `json:"location_type"`
	Quantity        int64  `json:"quantity"`
	TypeID          int64  `json:"type_id"`
}
type CharacterOrder struct {
	Duration      int       `json:"duration"`
	Escrow        float64   `json:"escrow"`
	IsBuyOrder    bool      `json:"is_buy_order"`
	IsCorporation bool      `json:"is_corporation"`
	Issued        time.Time `json:"issued"`
	LocationID    int64     `json:"location_id"`
	MinVolume     int64     `json:"min_volume"`
	OrderID       int64     `json:"order_id"`
	Price         float64   `json:"price"`
	Range         string    `json:"range"`
	RegionID      int64     `json:"region_id"`
	TypeID        int64     `json:"type_id"`
	VolumeRemain  int64     `json:"volume_remain"`
	VolumeTotal   int64     `json:"volume_total"`
}
type KillmailSummary struct {
	KillmailHash string `json:"killmail_hash"`
	KillmailID   int64  `json:"killmail_id"`
}
type Notification struct {
	IsRead         bool      `json:"is_read"`
	NotificationID int64     `json:"notification_id"`
	SenderID       int64     `json:"sender_id"`
	SenderType     string    `json:"sender_type"`
	Text           string    `json:"text"`
	Timestamp      time.Time `json:"timestamp"`
	Type           string    `json:"type"`
}
type CloneLocation struct {
	LocationID   int64  `json:"location_id"`
	LocationType string `json:"location_type"`
}
type CharacterClones struct {
	HomeLocation CloneLocation `json:"home_location"`
	JumpClones   []struct {
		Implants     []int64 `json:"implants"`
		JumpCloneID  int64   `json:"jump_clone_id"`
		LocationID   int64   `json:"location_id"`
		LocationType string  `json:"location_type"`
		Name         string  `json:"name"`
	} `json:"jump_clones"`
	LastCloneJumpDate     time.Time `json:"last_clone_jump_date"`
	LastStationChangeDate time.Time `json:"last_station_change_date"`
}
type CharacterFatigue struct {
	JumpFatigueExpireDate time.Time `json:"jump_fatigue_expire_date"`
	LastJumpDate          time.Time `json:"last_jump_date"`
	LastUpdateDate        time.Time `json:"last_update_date"`
}
type LoyaltyPoint struct {
	CorporationID int64 `json:"corporation_id"`
	LoyaltyPoints int64 `json:"loyalty_points"`
}
type Standing struct {
	FromID   int64   `json:"from_id"`
	FromType string  `json:"from_type"`
	Standing float64 `json:"standing"`
}
type CharacterTitle struct {
	Name    string `json:"name"`
	TitleID int64  `json:"title_id"`
}
type Blueprint struct {
	ItemID             int64  `json:"item_id"`
	LocationFlag       string `json:"location_flag"`
	LocationID         int64  `json:"location_id"`
	MaterialEfficiency int    `json:"material_efficiency"`
	Quantity           int64  `json:"quantity"`
	Runs               int    `json:"runs"`
	TimeEfficiency     int    `json:"time_efficiency"`
	TypeID             int64  `json:"type_id"`
}
type IndustryJob map[string]any
type MiningEntry map[string]any
type WalletTransaction map[string]any
type Contract map[string]any
type ContractItem map[string]any
type ContractBid map[string]any
type MailBody map[string]any
type KillmailDetail map[string]any
type CalendarEvent map[string]any
type Contact map[string]any
type ContactLabel map[string]any
type MailLabel map[string]any
type MailLabels struct {
	Labels           []MailLabel `json:"labels"`
	TotalUnreadCount int64       `json:"total_unread_count"`
}
type MailHeader map[string]any
type ChatChannel map[string]any
type AgentResearch map[string]any
type FWStats map[string]any
type Medal map[string]any
type Fitting map[string]any

func (g *Gateway) CharacterIdentity(ctx context.Context, characterID int64) (CharacterIdentity, Response, error) {
	var v CharacterIdentity
	if err := positive("character ID", characterID); err != nil {
		return v, Response{}, err
	}
	m, e := g.get(ctx, idPath("characters/", characterID, "/"), nil, "", &v)
	return v, m, e
}

func (g *Gateway) character(ctx context.Context, characterID int64, token, suffix string, target any) (Response, error) {
	if err := authenticated(characterID, token); err != nil {
		return Response{}, err
	}
	return g.get(ctx, idPath("characters/", characterID, suffix), nil, token, target)
}
func (g *Gateway) CharacterOnline(ctx context.Context, characterID int64, token string) (CharacterOnline, Response, error) {
	var v CharacterOnline
	m, e := g.character(ctx, characterID, token, "/online/", &v)
	return v, m, e
}
func (g *Gateway) CharacterLocation(ctx context.Context, characterID int64, token string) (CharacterLocation, Response, error) {
	var v CharacterLocation
	m, e := g.character(ctx, characterID, token, "/location/", &v)
	return v, m, e
}
func (g *Gateway) CharacterShip(ctx context.Context, characterID int64, token string) (CharacterShip, Response, error) {
	var v CharacterShip
	m, e := g.character(ctx, characterID, token, "/ship/", &v)
	return v, m, e
}
func (g *Gateway) CharacterSkills(ctx context.Context, characterID int64, token string) (CharacterSkills, Response, error) {
	var v CharacterSkills
	m, e := g.character(ctx, characterID, token, "/skills/", &v)
	return v, m, e
}
func (g *Gateway) CharacterSkillQueue(ctx context.Context, characterID int64, token string) ([]SkillQueueEntry, Response, error) {
	var v []SkillQueueEntry
	m, e := g.character(ctx, characterID, token, "/skillqueue/", &v)
	return v, m, e
}
func (g *Gateway) CharacterWallet(ctx context.Context, characterID int64, token string) (float64, Response, error) {
	var v float64
	m, e := g.character(ctx, characterID, token, "/wallet/", &v)
	return v, m, e
}
func (g *Gateway) CharacterWalletJournal(ctx context.Context, characterID int64, token string) ([]WalletJournalEntry, Response, error) {
	if e := authenticated(characterID, token); e != nil {
		return nil, Response{}, e
	}
	return getAllPages[WalletJournalEntry](ctx, g, idPath("characters/", characterID, "/wallet/journal/"), nil, token)
}
func (g *Gateway) CharacterAssets(ctx context.Context, characterID int64, token string) ([]Asset, Response, error) {
	if e := authenticated(characterID, token); e != nil {
		return nil, Response{}, e
	}
	return getAllPages[Asset](ctx, g, idPath("characters/", characterID, "/assets/"), nil, token)
}
func (g *Gateway) CharacterOrders(ctx context.Context, characterID int64, token string, includeHistorical bool) ([]CharacterOrder, Response, error) {
	if e := authenticated(characterID, token); e != nil {
		return nil, Response{}, e
	}
	suffix := "/orders/"
	if includeHistorical {
		suffix = "/orders/history/"
	}
	return getAllPages[CharacterOrder](ctx, g, idPath("characters/", characterID, suffix), nil, token)
}
func (g *Gateway) CharacterKillmails(ctx context.Context, characterID int64, token string, page int) ([]KillmailSummary, Response, error) {
	if e := authenticated(characterID, token); e != nil {
		return nil, Response{}, e
	}
	if page <= 0 {
		page = 1
	}
	var v []KillmailSummary
	m, e := g.get(ctx, idPath("characters/", characterID, "/killmails/recent/"), url.Values{"page": {strconv.Itoa(page)}}, token, &v)
	return v, m, e
}
func (g *Gateway) CharacterNotifications(ctx context.Context, characterID int64, token string) ([]Notification, Response, error) {
	var v []Notification
	m, e := g.character(ctx, characterID, token, "/notifications/", &v)
	return v, m, e
}
func characterValue[T any](ctx context.Context, g *Gateway, id int64, tok, suffix string) (T, Response, error) {
	var v T
	m, e := g.character(ctx, id, tok, suffix, &v)
	return v, m, e
}
func characterPages[T any](ctx context.Context, g *Gateway, id int64, tok, suffix string) ([]T, Response, error) {
	if e := authenticated(id, tok); e != nil {
		return nil, Response{}, e
	}
	return getAllPages[T](ctx, g, idPath("characters/", id, suffix), nil, tok)
}
func (g *Gateway) CharacterClones(c context.Context, id int64, t string) (CharacterClones, Response, error) {
	return characterValue[CharacterClones](c, g, id, t, "/clones/")
}
func (g *Gateway) CharacterImplants(c context.Context, id int64, t string) ([]int64, Response, error) {
	return characterValue[[]int64](c, g, id, t, "/implants/")
}
func (g *Gateway) CharacterFatigue(c context.Context, id int64, t string) (CharacterFatigue, Response, error) {
	return characterValue[CharacterFatigue](c, g, id, t, "/fatigue/")
}
func (g *Gateway) CharacterLoyalty(c context.Context, id int64, t string) ([]LoyaltyPoint, Response, error) {
	return characterValue[[]LoyaltyPoint](c, g, id, t, "/loyalty/points/")
}
func (g *Gateway) CharacterStandings(c context.Context, id int64, t string) ([]Standing, Response, error) {
	return characterValue[[]Standing](c, g, id, t, "/standings/")
}
func (g *Gateway) CharacterTitles(c context.Context, id int64, t string) ([]CharacterTitle, Response, error) {
	return characterValue[[]CharacterTitle](c, g, id, t, "/titles/")
}
func (g *Gateway) CharacterBlueprints(c context.Context, id int64, t string) ([]Blueprint, Response, error) {
	return characterPages[Blueprint](c, g, id, t, "/blueprints/")
}
func (g *Gateway) CharacterIndustryJobs(c context.Context, id int64, t string) ([]IndustryJob, Response, error) {
	return characterValue[[]IndustryJob](c, g, id, t, "/industry/jobs/")
}
func (g *Gateway) CharacterMining(c context.Context, id int64, t string) ([]MiningEntry, Response, error) {
	return characterValue[[]MiningEntry](c, g, id, t, "/mining/")
}
func (g *Gateway) CharacterWalletTransactions(c context.Context, id int64, t string) ([]WalletTransaction, Response, error) {
	return characterPages[WalletTransaction](c, g, id, t, "/wallet/transactions/")
}
func (g *Gateway) CharacterContracts(c context.Context, id int64, t string) ([]Contract, Response, error) {
	return characterPages[Contract](c, g, id, t, "/contracts/")
}
func (g *Gateway) CharacterContractItems(c context.Context, id, contractID int64, t string) ([]ContractItem, Response, error) {
	if contractID <= 0 {
		return nil, Response{}, errors.New("contract ID must be positive")
	}
	return characterValue[[]ContractItem](c, g, id, t, "/contracts/"+strconv.FormatInt(contractID, 10)+"/items/")
}
func (g *Gateway) CharacterContractBids(c context.Context, id, contractID int64, t string) ([]ContractBid, Response, error) {
	if contractID <= 0 {
		return nil, Response{}, errors.New("contract ID must be positive")
	}
	return characterValue[[]ContractBid](c, g, id, t, "/contracts/"+strconv.FormatInt(contractID, 10)+"/bids/")
}
func (g *Gateway) CharacterMailBody(c context.Context, id, mailID int64, t string) (MailBody, Response, error) {
	if mailID <= 0 {
		return nil, Response{}, errors.New("mail ID must be positive")
	}
	return characterValue[MailBody](c, g, id, t, "/mail/"+strconv.FormatInt(mailID, 10)+"/")
}
func (g *Gateway) KillmailDetail(c context.Context, killmailID int64, hash string) (KillmailDetail, Response, error) {
	if killmailID <= 0 || len(hash) < 20 || len(hash) > 128 {
		return nil, Response{}, errors.New("killmail reference is invalid")
	}
	for _, r := range hash {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return nil, Response{}, errors.New("killmail reference is invalid")
		}
	}
	var v KillmailDetail
	m, e := g.get(c, "killmails/"+strconv.FormatInt(killmailID, 10)+"/"+hash+"/", nil, "", &v)
	return v, m, e
}
func (g *Gateway) CharacterCalendar(c context.Context, id int64, t string) ([]CalendarEvent, Response, error) {
	return characterValue[[]CalendarEvent](c, g, id, t, "/calendar/")
}
func (g *Gateway) CharacterContacts(c context.Context, id int64, t string) ([]Contact, Response, error) {
	return characterPages[Contact](c, g, id, t, "/contacts/")
}
func (g *Gateway) CharacterContactLabels(c context.Context, id int64, t string) ([]ContactLabel, Response, error) {
	return characterValue[[]ContactLabel](c, g, id, t, "/contacts/labels/")
}
func (g *Gateway) CharacterMailLabels(c context.Context, id int64, t string) (MailLabels, Response, error) {
	return characterValue[MailLabels](c, g, id, t, "/mail/labels/")
}
func (g *Gateway) CharacterMailHeaders(c context.Context, id int64, t string) ([]MailHeader, Response, error) {
	return characterValue[[]MailHeader](c, g, id, t, "/mail/")
}
func (g *Gateway) CharacterChatChannels(c context.Context, id int64, t string) ([]ChatChannel, Response, error) {
	return characterValue[[]ChatChannel](c, g, id, t, "/chat_channels/")
}
func (g *Gateway) CharacterResearch(c context.Context, id int64, t string) ([]AgentResearch, Response, error) {
	return characterValue[[]AgentResearch](c, g, id, t, "/agents_research/")
}
func (g *Gateway) CharacterFWStats(c context.Context, id int64, t string) (FWStats, Response, error) {
	return characterValue[FWStats](c, g, id, t, "/fw/stats/")
}
func (g *Gateway) CharacterMedals(c context.Context, id int64, t string) ([]Medal, Response, error) {
	return characterValue[[]Medal](c, g, id, t, "/medals/")
}
func (g *Gateway) CharacterFittings(c context.Context, id int64, t string) ([]Fitting, Response, error) {
	return characterValue[[]Fitting](c, g, id, t, "/fittings/")
}
