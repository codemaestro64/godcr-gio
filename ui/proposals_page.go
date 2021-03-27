package ui

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ararog/timeago"
	"github.com/planetdecred/dcrlibwallet"
	"github.com/planetdecred/godcr/ui/decredmaterial"
	"github.com/planetdecred/godcr/ui/values"
	"github.com/planetdecred/godcr/wallet"
)

const PageProposals = "Proposals"
const (
	categoryStateFetching = iota
	categoryStateFetched
	categoryStateError
)

type proposalNotificationListeners struct {
	page *proposalsPage
}

func (p proposalNotificationListeners) OnNewProposal(proposal *dcrlibwallet.Proposal) {
	p.page.addDiscoveredProposal(*proposal)
}

func (p proposalNotificationListeners) OnProposalVoteStarted(proposal *dcrlibwallet.Proposal) {
	p.page.updateProposal(*proposal)
}

func (p proposalNotificationListeners) OnProposalVoteFinished(proposal *dcrlibwallet.Proposal) {
	p.page.updateProposal(*proposal)
}

func (p proposalNotificationListeners) OnProposalsSynced() {
	p.page.isSynced = true
	p.page.refreshWindow()
}

type proposalItem struct {
	btn      *widget.Clickable
	proposal dcrlibwallet.Proposal
	voteBar  decredmaterial.VoteBar
}

type tab struct {
	title     string
	btn       *widget.Clickable
	category  int32
	proposals []proposalItem
	container *decredmaterial.ScrollContainer
}

type tabs struct {
	tabs     []tab
	selected int
}

type proposalsPage struct {
	theme                      *decredmaterial.Theme
	wallet                     *wallet.Wallet
	proposalsList              *layout.List
	scrollContainer            *decredmaterial.ScrollContainer
	tabs                       tabs
	tabCard                    decredmaterial.Card
	itemCard                   decredmaterial.Card
	syncCard                   decredmaterial.Card
	notify                     func(string, bool)
	hasFetchedInitialProposals bool
	isFetchingInitialProposals bool
	legendIcon                 *widget.Icon
	infoIcon                   *widget.Icon
	selectedProposal           **dcrlibwallet.Proposal
	state                      int
	hasRegisteredListeners     bool
	isSynced                   bool
	refreshWindow              func()
	updatedIcon                *widget.Icon
	updatedLabel               decredmaterial.Label
	syncIcon                   image.Image
	syncButton                 *widget.Clickable
	startSyncIcon              *widget.Image
}

var (
	proposalCategoryTitles = []string{"In discussion", "Voting", "Approved", "Rejected", "Abandoned"}
	proposalCategories     = []int32{
		dcrlibwallet.ProposalCategoryPre,
		dcrlibwallet.ProposalCategoryActive,
		dcrlibwallet.ProposalCategoryApproved,
		dcrlibwallet.ProposalCategoryRejected,
		dcrlibwallet.ProposalCategoryAbandoned,
	}
)

func (win *Window) ProposalsPage(common pageCommon) layout.Widget {
	pg := &proposalsPage{
		theme:            common.theme,
		wallet:           win.wallet,
		proposalsList:    &layout.List{},
		scrollContainer:  common.theme.ScrollContainer(),
		tabCard:          common.theme.Card(),
		itemCard:         common.theme.Card(),
		syncCard:         common.theme.Card(),
		notify:           common.notify,
		legendIcon:       common.icons.imageBrightness1,
		infoIcon:         common.icons.actionInfo,
		selectedProposal: &win.selectedProposal,
		refreshWindow:    common.refreshWindow,
		updatedIcon:      common.icons.navigationCheck,
		updatedLabel:     common.theme.Body2("Updated"),
		syncIcon:         common.icons.syncingIcon,
		syncButton:       new(widget.Clickable),
		startSyncIcon:    common.icons.restore,
	}
	pg.updatedIcon.Color = common.theme.Color.Success
	pg.updatedLabel.Color = common.theme.Color.Success

	pg.tabCard.Radius = decredmaterial.CornerRadius{NE: 0, NW: 0, SE: 0, SW: 0}
	pg.syncCard.Radius = decredmaterial.CornerRadius{NE: 0, NW: 0, SE: 0, SW: 0}

	for i := range proposalCategoryTitles {
		pg.tabs.tabs = append(pg.tabs.tabs,
			tab{
				title:     proposalCategoryTitles[i],
				btn:       new(widget.Clickable),
				category:  proposalCategories[i],
				container: pg.theme.ScrollContainer(),
			},
		)
	}

	return func(gtx C) D {
		pg.Handle(common)
		return pg.Layout(gtx, common)
	}
}

func (pg *proposalsPage) Handle(common pageCommon) {
	for i := range pg.tabs.tabs {
		if pg.tabs.tabs[i].btn.Clicked() {
			pg.tabs.selected = i
		}

		for k := range pg.tabs.tabs[i].proposals {
			for pg.tabs.tabs[i].proposals[k].btn.Clicked() {
				*pg.selectedProposal = &pg.tabs.tabs[i].proposals[k].proposal
				common.ChangePage(PageProposalDetails)
			}
		}
	}

	for pg.syncButton.Clicked() {
		pg.wallet.SyncProposals()
		pg.refreshWindow()
	}
}

func (pg *proposalsPage) addDiscoveredProposal(proposal dcrlibwallet.Proposal) {
	for i := range pg.tabs.tabs {
		if pg.tabs.tabs[i].category == proposal.Category {
			item := proposalItem{
				btn:      new(widget.Clickable),
				proposal: proposal,
				voteBar:  pg.theme.VoteBar(pg.infoIcon, pg.legendIcon),
			}
			pg.tabs.tabs[i].proposals = append([]proposalItem{item}, pg.tabs.tabs[i].proposals...)
			break
		}
	}
}

func (pg *proposalsPage) updateProposal(proposal dcrlibwallet.Proposal) {
out:
	for i := range pg.tabs.tabs {
		for k := range pg.tabs.tabs[i].proposals {
			if pg.tabs.tabs[i].proposals[k].proposal.Token == proposal.Token {
				pg.tabs.tabs[i].proposals = append(pg.tabs.tabs[i].proposals[:k], pg.tabs.tabs[i].proposals[k+1:]...)
				break out
			}
		}
	}
	pg.addDiscoveredProposal(proposal)
}

func (pg *proposalsPage) onFetchSuccess(proposals []dcrlibwallet.Proposal) {
	for i := range proposals {
		item := proposalItem{
			btn:      new(widget.Clickable),
			proposal: proposals[i],
			voteBar:  pg.theme.VoteBar(pg.infoIcon, pg.legendIcon),
		}

		for k := range pg.tabs.tabs {
			if pg.tabs.tabs[k].category == proposals[i].Category {
				pg.tabs.tabs[k].proposals = append(pg.tabs.tabs[k].proposals, item)
				break
			}
		}
	}
	pg.onFetchComplete()
	pg.state = categoryStateFetched
}

func (pg *proposalsPage) onFetchError(err error) {
	pg.state = categoryStateError
	pg.onFetchComplete()
	pg.notify(err.Error(), false)
}

func (pg *proposalsPage) onFetchComplete() {
	if !pg.hasFetchedInitialProposals {
		pg.hasFetchedInitialProposals = true
	}

	if !pg.isFetchingInitialProposals {
		pg.isFetchingInitialProposals = false
	}
}

func (pg *proposalsPage) fetchProposals() {
	pg.isFetchingInitialProposals = true
	pg.wallet.GetProposals(dcrlibwallet.ProposalCategoryAll, pg.onFetchSuccess, pg.onFetchError)
}

func (pg *proposalsPage) layoutTabs(gtx C) D {
	width := float32(gtx.Constraints.Max.X-20) / float32(len(pg.tabs.tabs))

	return pg.tabCard.Layout(gtx, func(gtx C) D {
		return layout.Inset{
			Left:  values.MarginPadding12,
			Right: values.MarginPadding12,
		}.Layout(gtx, func(gtx C) D {
			return pg.proposalsList.Layout(gtx, len(pg.tabs.tabs), func(gtx C, i int) D {
				gtx.Constraints.Min.X = int(width)
				return layout.Stack{Alignment: layout.S}.Layout(gtx,
					layout.Stacked(func(gtx C) D {
						return decredmaterial.Clickable(gtx, pg.tabs.tabs[i].btn, func(gtx C) D {
							return layout.UniformInset(values.MarginPadding14).Layout(gtx, func(gtx C) D {
								return layout.Center.Layout(gtx, func(gtx C) D {
									return layout.Flex{}.Layout(gtx,
										layout.Rigid(func(gtx C) D {
											lbl := pg.theme.Body1(pg.tabs.tabs[i].title)
											lbl.Color = pg.theme.Color.Gray
											if pg.tabs.selected == i {
												lbl.Color = pg.theme.Color.Primary
											}
											return lbl.Layout(gtx)
										}),
										layout.Rigid(func(gtx C) D {
											return layout.Inset{Left: values.MarginPadding4, Top: values.MarginPadding2}.Layout(gtx, func(gtx C) D {
												c := pg.theme.Card()
												c.Color = pg.theme.Color.LightGray
												r := float32(8.5)
												c.Radius = decredmaterial.CornerRadius{NE: r, NW: r, SE: r, SW: r}
												lbl := pg.theme.Body2(strconv.Itoa(len(pg.tabs.tabs[i].proposals)))
												lbl.Color = pg.theme.Color.IconColor
												if pg.tabs.selected == i {
													c.Color = pg.theme.Color.Primary
													lbl.Color = pg.theme.Color.Surface
												}
												return c.Layout(gtx, func(gtx C) D {
													return layout.Inset{Left: values.MarginPadding5, Right: values.MarginPadding5}.Layout(gtx, func(gtx C) D {
														return lbl.Layout(gtx)
													})
												})
											})
										}),
									)
								})
							})
						})
					}),
					layout.Stacked(func(gtx C) D {
						if pg.tabs.selected != i {
							return D{}
						}
						tabHeight := gtx.Px(unit.Dp(2))
						tabRect := image.Rect(0, 0, int(width), tabHeight)
						paint.FillShape(gtx.Ops, pg.theme.Color.Primary, clip.Rect(tabRect).Op())
						return layout.Dimensions{
							Size: image.Point{X: int(width), Y: tabHeight},
						}
					}),
				)
			})
		})
	})
}

func (pg *proposalsPage) layoutFetchingState(gtx C) D {
	str := "Fetching " + strings.ToLower(proposalCategoryTitles[pg.tabs.selected]) + " proposals..."
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Center.Layout(gtx, func(gtx C) D {
		return pg.theme.Body1(str).Layout(gtx)
	})
}

func (pg *proposalsPage) layoutErrorState(gtx C) D {
	str := "Error fetching proposals"

	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Center.Layout(gtx, func(gtx C) D {
		lbl := pg.theme.Body1(str)
		lbl.Color = pg.theme.Color.Danger
		return lbl.Layout(gtx)
	})
}

func (pg *proposalsPage) layoutNoProposalsFound(gtx C) D {
	str := "No " + strings.ToLower(proposalCategoryTitles[pg.tabs.selected]) + " proposals"

	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Center.Layout(gtx, func(gtx C) D {
		return pg.theme.Body1(str).Layout(gtx)
	})
}

func (pg *proposalsPage) layoutAuthorAndDate(gtx C, proposal dcrlibwallet.Proposal) D {
	nameLabel := pg.theme.Body2(proposal.Username)
	dotLabel := pg.theme.H4(" . ")
	versionLabel := pg.theme.Body2("Version " + proposal.Version)
	timeAgoLabel := pg.theme.Body2(timeAgo(proposal.Timestamp))

	var categoryLabel decredmaterial.Label
	var categoryLabelColor color.NRGBA
	switch proposal.Category {
	case dcrlibwallet.ProposalCategoryApproved:
		categoryLabel = pg.theme.Body2("Approved")
		categoryLabelColor = pg.theme.Color.Success
	case dcrlibwallet.ProposalCategoryActive:
		categoryLabel = pg.theme.Body2("Voting")
		categoryLabelColor = pg.theme.Color.Primary
	case dcrlibwallet.ProposalCategoryRejected:
		categoryLabel = pg.theme.Body2("Rejected")
		categoryLabelColor = pg.theme.Color.Danger
	case dcrlibwallet.ProposalCategoryAbandoned:
		categoryLabel = pg.theme.Body2("Abandoned")
		categoryLabelColor = pg.theme.Color.Gray
	case dcrlibwallet.ProposalCategoryPre:
		categoryLabel = pg.theme.Body2("in discussion")
		categoryLabelColor = pg.theme.Color.Gray
	}
	categoryLabel.Color = categoryLabelColor

	return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(nameLabel.Layout),
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(-23)}.Layout(gtx, dotLabel.Layout)
				}),
				layout.Rigid(versionLabel.Layout),
			)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(categoryLabel.Layout),
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(-23)}.Layout(gtx, dotLabel.Layout)
				}),
				layout.Rigid(timeAgoLabel.Layout),
			)
		}),
	)
}

func (pg *proposalsPage) layoutTitle(gtx C, proposal dcrlibwallet.Proposal) D {
	lbl := pg.theme.H6(proposal.Name)
	lbl.Color = pg.theme.Color.Text

	return layout.Inset{
		Top:    values.MarginPadding5,
		Bottom: values.MarginPadding5,
	}.Layout(gtx, lbl.Layout)
}

func (pg *proposalsPage) layoutProposalVoteBar(gtx C, proposalItem proposalItem) D {
	yes := float32(proposalItem.proposal.YesVotes)
	no := float32(proposalItem.proposal.NoVotes)
	quorumPercent := float32(proposalItem.proposal.QuorumPercentage)
	passPercentage := float32(proposalItem.proposal.PassPercentage)
	eligibleTickets := float32(proposalItem.proposal.EligibleTickets)

	return proposalItem.voteBar.SetParams(yes, no, eligibleTickets, quorumPercent, passPercentage).LayoutWithLegend(gtx)
}

func (pg *proposalsPage) layoutProposalsList(gtx C) D {
	selected := pg.tabs.tabs[pg.tabs.selected]
	wdgs := make([]func(gtx C) D, len(selected.proposals))
	for i := range selected.proposals {
		index := i
		proposalItem := selected.proposals[index]
		wdgs[index] = func(gtx C) D {
			return layout.Inset{
				Top:    values.MarginPadding5,
				Bottom: values.MarginPadding5,
				Left:   values.MarginPadding15,
				Right:  values.MarginPadding15,
			}.Layout(gtx, func(gtx C) D {
				return decredmaterial.Clickable(gtx, selected.proposals[index].btn, func(gtx C) D {
					return pg.itemCard.Layout(gtx, func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.UniformInset(values.MarginPadding15).Layout(gtx, func(gtx C) D {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									return pg.layoutAuthorAndDate(gtx, proposalItem.proposal)
								}),
								layout.Rigid(func(gtx C) D {
									return pg.layoutTitle(gtx, proposalItem.proposal)
								}),
								layout.Rigid(func(gtx C) D {
									if proposalItem.proposal.Category == dcrlibwallet.ProposalCategoryActive ||
										proposalItem.proposal.Category == dcrlibwallet.ProposalCategoryApproved ||
										proposalItem.proposal.Category == dcrlibwallet.ProposalCategoryRejected {
										return pg.layoutProposalVoteBar(gtx, proposalItem)
									}
									return D{}
								}),
							)
						})
					})
				})
			})
		}
	}
	return selected.container.Layout(gtx, wdgs)
}

func (pg *proposalsPage) layoutContent(gtx C) D {
	selected := pg.tabs.tabs[pg.tabs.selected]
	if pg.state == categoryStateFetching {
		return pg.layoutFetchingState(gtx)
	} else if pg.state == categoryStateFetched && len(selected.proposals) == 0 {
		return pg.layoutNoProposalsFound(gtx)
	} else if pg.state == categoryStateError {
		return pg.layoutErrorState(gtx)
	}
	return pg.layoutProposalsList(gtx)
}

func (pg *proposalsPage) layoutIsSyncedSection(gtx C) D {
	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return pg.updatedIcon.Layout(gtx, values.MarginPadding20)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: values.MarginPadding5}.Layout(gtx, pg.updatedLabel.Layout)
		}),
	)
}

func (pg *proposalsPage) layoutIsSyncingSection(gtx C) D {
	txt := pg.theme.Body2("Fetching...")
	txt.Color = pg.theme.Color.Gray
	return txt.Layout(gtx)
}

func (pg *proposalsPage) layoutStartSyncSection(gtx C) D {
	return material.Clickable(gtx, pg.syncButton, func(gtx C) D {
		pg.startSyncIcon.Scale = 0.68
		return pg.startSyncIcon.Layout(gtx)
	})
}

func (pg *proposalsPage) layoutSyncSection(gtx C) D {
	if pg.isSynced {
		return pg.layoutIsSyncedSection(gtx)
	} else if pg.wallet.IsSyncingPropoals() {
		return pg.layoutIsSyncingSection(gtx)
	}
	return pg.layoutStartSyncSection(gtx)
}

func (pg *proposalsPage) Layout(gtx C, common pageCommon) D {
	if !pg.hasFetchedInitialProposals && !pg.isFetchingInitialProposals {
		pg.fetchProposals()
	}

	if !pg.hasRegisteredListeners {
		pg.wallet.AddProposalNotificationListener(proposalNotificationListeners{pg})
		pg.hasRegisteredListeners = true
	}

	border := widget.Border{Color: pg.theme.Color.BorderColor, CornerRadius: values.MarginPadding0, Width: values.MarginPadding1}
	borderLayout := func(gtx layout.Context, body layout.Widget) layout.Dimensions {
		return border.Layout(gtx, body)
	}

	return common.LayoutWithoutPadding(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						return borderLayout(gtx, pg.layoutTabs)
					}),
					layout.Rigid(func(gtx C) D {
						return borderLayout(gtx, func(gtx C) D {
							return pg.syncCard.Layout(gtx, func(gtx C) D {
								m := values.MarginPadding12
								if pg.isSynced {
									m = values.MarginPadding14
								} else if pg.wallet.IsSyncingPropoals() {
									m = values.MarginPadding15
								}
								return layout.UniformInset(m).Layout(gtx, func(gtx C) D {
									return layout.Center.Layout(gtx, func(gtx C) D {
										return pg.layoutSyncSection(gtx)
									})
								})
							})
						})
					}),
				)
			}),
			layout.Flexed(1, pg.layoutContent),
		)
	})
}

func timeAgo(timestamp int64) string {
	timeAgo, _ := timeago.TimeAgoWithTime(time.Now(), time.Unix(timestamp, 0))
	return timeAgo
}

func truncateString(str string, num int) string {
	bnoden := str
	if len(str) > num {
		if num > 3 {
			num -= 3
		}
		bnoden = str[0:num] + "..."
	}
	return bnoden
}
