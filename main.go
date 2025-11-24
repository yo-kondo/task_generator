/*
# タスク一覧の繰り返しタスク作成
*/

package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ---------------------------------------------------------
// データ構造定義
// ---------------------------------------------------------

// HolidayConfig は holiday.toml の構造を定義
type HolidayConfig struct {
	Holidays []Holiday `toml:"holiday"`
}

type Holiday struct {
	Name        string `toml:"name"`
	Date        string `toml:"holiday"`
	HolidayNote string `toml:"holiday_note"`
}

// TaskConfig は task.toml の構造を定義
type TaskConfig struct {
	Tasks []TaskTemplate `toml:"task"`
}

type TaskTemplate struct {
	Repeat        string `toml:"repeat"`
	Time          string `toml:"time"`
	EstimatedTime string `toml:"estimated_time"`
	Project       string `toml:"project"`
	TaskName      string `toml:"task_name"`
	TaskNote      string `toml:"task_note"`
}

// OutputRow は出力する1行分のデータを保持
type OutputRow struct {
	Date          time.Time
	Time          string
	EstimatedTime string
	ActualTime    string // 常に空白
	Project       string
	TaskName      string
	TaskNote      string
}

// ---------------------------------------------------------
// グローバル定数・変数
// ---------------------------------------------------------

// 曜日文字列と time.Weekday のマッピング
var weekdayMap = map[string]time.Weekday{
	"日曜日": time.Sunday,
	"月曜日": time.Monday,
	"火曜日": time.Tuesday,
	"水曜日": time.Wednesday,
	"木曜日": time.Thursday,
	"金曜日": time.Friday,
	"土曜日": time.Saturday,
}

// 祝日判定用マップ (key: yyyy-mm-dd)
var holidayMap map[string]bool

// ---------------------------------------------------------
// メイン処理
// ---------------------------------------------------------

func main() {
	// 1. コマンドライン引数のチェック
	if len(os.Args) < 2 {
		log.Fatal("エラー: 引数に年月(yyyymm)を指定してください。\n例: go run main.go 202511")
	}
	targetYMStr := os.Args[1]

	// 年月のパース (yyyymm -> time.Time)
	targetDate, err := time.Parse("200601", targetYMStr)
	if err != nil {
		log.Fatalf("エラー: 年月の形式が不正です(yyyymm形式で指定してください): %v", err)
	}

	// 2. 設定ファイルの読み込み
	loadHolidays("holiday.toml")
	tasks := loadTasks("task.toml")

	// 3. タスクの生成処理
	var outputRows []OutputRow

	// 月の初日と最終日を計算
	firstDay := targetDate
	lastDay := firstDay.AddDate(0, 1, -1)

	for _, tmpl := range tasks {
		generatedDates := []time.Time{}

		switch {
		case tmpl.Repeat == "毎日":
			// 毎日：土日祝を除く平日
			for d := firstDay; !d.After(lastDay); d = d.AddDate(0, 0, 1) {
				if isBusinessDay(d) {
					generatedDates = append(generatedDates, d)
				}
			}

		case tmpl.Repeat == "月初":
			// 月初：月の初めの平日
			d := firstDay
			// 平日が見つかるまで進める
			for !d.After(lastDay) {
				if isBusinessDay(d) {
					generatedDates = append(generatedDates, d)
					break
				}
				d = d.AddDate(0, 0, 1)
			}

		case tmpl.Repeat == "月末":
			// 月末：月の最後の平日
			d := lastDay
			// 平日が見つかるまで戻る
			for !d.Before(firstDay) {
				if isBusinessDay(d) {
					generatedDates = append(generatedDates, d)
					break
				}
				d = d.AddDate(0, 0, -1)
			}

		case strings.HasPrefix(tmpl.Repeat, "毎週"):
			// 毎週◯曜日
			targetWeekdayStr := strings.TrimPrefix(tmpl.Repeat, "毎週")
			targetWeekday, ok := weekdayMap[targetWeekdayStr]
			if !ok {
				log.Printf("警告: 未知の曜日指定です: %s (タスク: %s)", tmpl.Repeat, tmpl.TaskName)
				continue
			}

			// 月内の指定曜日を全て探す
			for d := firstDay; !d.After(lastDay); d = d.AddDate(0, 0, 1) {
				if d.Weekday() == targetWeekday {
					// 祝日チェック
					finalDate := d
					if isHoliday(d) {
						// 祝日の場合、1日前の平日を探す
						// ※前日が前月になる場合でも、その日付でタスクを作成するのが一般的と判断
						finalDate = getPreviousBusinessDay(d)
					}
					generatedDates = append(generatedDates, finalDate)
				}
			}

		default:
			log.Fatalf("エラー: 未対応の繰り返し設定です: %s (タスク: %s)", tmpl.Repeat, tmpl.TaskName)
		}

		// 生成された日付ごとに出力行を作成
		for _, date := range generatedDates {
			row := OutputRow{
				Date:          date,
				Time:          tmpl.Time,
				EstimatedTime: tmpl.EstimatedTime,
				ActualTime:    "",
				Project:       tmpl.Project,
				TaskName:      tmpl.TaskName,
				TaskNote:      tmpl.TaskNote,
			}
			outputRows = append(outputRows, row)
		}
	}

	// 4. 日付順にソート
	sort.Slice(outputRows, func(i, j int) bool {
		// 日付が同じなら時刻で比較、それも同じならタスク名で比較（安定ソートのため）
		if outputRows[i].Date.Equal(outputRows[j].Date) {
			if outputRows[i].Time == outputRows[j].Time {
				return outputRows[i].TaskName < outputRows[j].TaskName
			}
			return outputRows[i].Time < outputRows[j].Time
		}
		return outputRows[i].Date.Before(outputRows[j].Date)
	})

	// 5. 出力
	for _, row := range outputRows {
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Date.Format("2006-01-02"),
			row.Time,
			row.EstimatedTime,
			row.ActualTime,
			row.Project,
			row.TaskName,
			row.TaskNote,
		)
	}
}

// ---------------------------------------------------------
// ヘルパー関数
// ---------------------------------------------------------

// loadHolidays は holiday.toml を読み込み map に格納する
func loadHolidays(filename string) {
	var config HolidayConfig
	if _, err := toml.DecodeFile(filename, &config); err != nil {
		log.Fatalf("エラー: 祝日ファイル(%s)の読み込みに失敗しました: %v", filename, err)
	}

	holidayMap = make(map[string]bool)
	for _, h := range config.Holidays {
		// 念のため日付フォーマットを正規化またはチェックすることも可能
		holidayMap[h.Date] = true
	}
}

// loadTasks は task.toml を読み込み構造体のスライスを返す
func loadTasks(filename string) []TaskTemplate {
	var config TaskConfig
	if _, err := toml.DecodeFile(filename, &config); err != nil {
		log.Fatalf("エラー: タスクファイル(%s)の読み込みに失敗しました: %v", filename, err)
	}
	return config.Tasks
}

// isHoliday は指定日が祝日かどうかを判定する
func isHoliday(t time.Time) bool {
	dateStr := t.Format("2006-01-02")
	return holidayMap[dateStr]
}

// isWeekend は指定日が土日かどうかを判定する
func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// isBusinessDay は平日（土日祝以外）かどうかを判定する
func isBusinessDay(t time.Time) bool {
	return !isWeekend(t) && !isHoliday(t)
}

// getPreviousBusinessDay は指定日の「1日前の平日」を再帰的/ループで探索する
func getPreviousBusinessDay(baseDate time.Time) time.Time {
	d := baseDate.AddDate(0, 0, -1) // まず1日戻る
	for {
		if isBusinessDay(d) {
			return d
		}
		// 土日祝ならさらに戻る
		d = d.AddDate(0, 0, -1)
	}
}
