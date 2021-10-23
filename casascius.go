package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/gosuri/uilive"
	"github.com/jbenet/go-base58"

	//boom "github.com/tylertreat/BoomFilters"

	boom "github.com/bits-and-blooms/bloom/v3"
	//rpcclient "github.com/stevenroose/go-bitcoin-core-rpc"
	"golang.org/x/crypto/ripemd160"

	_ "github.com/mattn/go-sqlite3"
)

var walletList_opt *string = flag.String("w", "wallets.txt", "wallets.txt")
var walletInsert_opt *bool = flag.Bool("wi", false, "true")
var phraseCount_opt *int = flag.Int("pc", 29, "29")
var input_opt *string = flag.String("i", "phrases.txt", "phrases.txt")
var output_opt *string = flag.String("o", "bingo.txt", "bingo.txt")
var thread_opt *int = flag.Int("t", 4, "1")
var verbose_opt *bool = flag.Bool("v", false, "false")

var addressList, wordList []string

type Wallet struct {
	addressCompressed   string
	addressUncompressed string
	passphrase          string
}

var botStartElapsed time.Time

//var start time.Time
var writer *uilive.Writer
var wordlistCount int = 0
var wordlistIndex int = 0
var total uint64 = 0
var totalFound uint64 = 0
var totalBalancedAddress uint64 = 0
var BalanceAPI string = "https://sochain.com/api/v2/get_address_balance/bitcoin/" //API

var sqliteDatabase *sql.DB

var sbf *boom.BloomFilter

func main() {
	flag.Parse()

	wordListRead, _ := ioutil.ReadFile(*input_opt)
	file_content := string(wordListRead)
	wordList = strings.Split(strings.Replace(file_content, "\r\n", "\n", -1), "\n")

	bytesRead, _ := ioutil.ReadFile(*walletList_opt)
	file_content = string(bytesRead)
	addressList = strings.Split(strings.Replace(file_content, "\r\n", "\n", -1), "\n")
	//fmt.Println(wordList)
	//fmt.Println(addressList)
	addressCount := len(addressList)

	fmt.Println("_____Parameter Settings_____")
	fmt.Println("Address:", *walletList_opt)
	fmt.Println("Address Insert:", *walletInsert_opt)
	fmt.Println("Phrase Count:", *phraseCount_opt)
	fmt.Println("Input:", *input_opt)
	fmt.Println("Output:", *output_opt)
	fmt.Println("Thread:", *thread_opt)
	fmt.Println("Log Output:", *verbose_opt)
	fmt.Println("_____Info_____")

	fmt.Println("Wordlist Count:", len(wordList))
	fmt.Println("Total Address:", uint(addressCount))

	sbf = boom.NewWithEstimates(uint(addressCount), 0.0000001) //0.00000000000000000001  0.0000001

	for _, address := range addressList {
		sbf.Add([]byte(address))
	}
	fmt.Println("Wallets Loaded")

	wordlistCount = len(wordList)

	// //_____________Bloom Test
	// var testFind uint64 = 0
	// var testNoFind uint64 = 0
	// for _, adres := range addressList {
	// 	if sbf.Test([]byte(adres)) {
	// 		testFind++
	// 	} else {
	// 		testNoFind++
	// 	}
	// }
	// fmt.Println("Total:", len(addressList))
	// fmt.Println("FindTotal:", testFind)
	// fmt.Println("NoFind:", testNoFind)
	// //_____________Bloom Test

	//GeneratorFull("SymcR374ukWS48XCfENrDPGaYagFpG")
	//os.Exit(0)

	// if sbf.Test([]byte(AddressToRIPEM160(`15Jp2rPA5zbEZV4rwCSLufreJWyJUfyA58`))) {
	// 	fmt.Println("contains b")
	// }

	if *walletInsert_opt {
		createDB()
	}
	sqliteDatabase, _ = sql.Open("sqlite3", "database.db") // Open the created SQLite File
	defer sqliteDatabase.Close()                           // Defer Closing the database
	if *walletInsert_opt {
		InsertWalletToDB()
	}
	go Counter() //Stat
	// var wg sync.WaitGroup
	// for i := 1; i <= *thread_opt; i++ {
	// 	wg.Add(1)
	// 	go Brute(i, &wg)
	// }
	// wg.Wait()

	// if CheckWallet(sqliteDatabase, "1JiFuXz6PgbDbxwg7ptFxem7iAeyJwR9vx") { //Elapsed Time 0.0010004
	// 	fmt.Println("Buldum")
	// } else {
	// 	fmt.Println("Bulamadım")
	// }
	//os.Exit(0)

	var wg sync.WaitGroup

	/*
	 * Tell the 'wg' WaitGroup how many threads/goroutines
	 *  that are about to run concurrently.
	 */
	wg.Add(*thread_opt)

	fmt.Println("Threads Starting")
	for i := 0; i < *thread_opt; i++ {

		/*
		 * Spawn a thread for each iteration in the loop.
		 * Pass 'i' into goroutine's function
		 * in order to make sure each goroutine use a different value for 'i'
		 */
		go func(i int) {
			// At the end of the goroutine, tell the WaitGroup that another thread has completed
			defer wg.Done()
			Brute(i, &wg)
			fmt.Printf("i: %v\n", i)
		}(i)
	}
	wg.Wait()
	fmt.Println("Threads Started")

	//Close Function
	sig := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sig
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()
	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
	//Close Function
}

func init() {
	botStartElapsed = time.Now()
}

func Counter() {
	writer = uilive.New()
	writer.Start()
	time.Sleep(time.Millisecond * 1000) //1 Second
	for {
		avgSpeed := total / uint64(time.Since(botStartElapsed).Seconds())
		fmt.Fprintf(writer, "Total Wallet = %v\nThread Count = %v\nElapsed Time = %v\nGenerated Wallet = %d\nGenerate Speed Avg(s) = %v\nFound = %d\nTotal Balanced Address = %d\nFor Close ctrl+c\n", len(addressList), *thread_opt, time.Since(botStartElapsed).String(), total, avgSpeed, totalFound, totalBalancedAddress)
		time.Sleep(time.Millisecond * 1000) //1 Second
	}
	//writer.Stop() // flush and stop rendering
}

func Brute(id int, wg *sync.WaitGroup) {
	//fmt.Println(id)
	defer wg.Done()
	for { ////Elapsed Time 0.0010003

		var randomPhrase string

		randomPhrase = RandomPhrase(*phraseCount_opt) //Elapsed Time 0.000999

		randomWallet := GeneratorFull(randomPhrase) //Elapsed Time 0.0010002
		//fmt.Println(randomWallet.base58BitcoinAddress)

		if sbf.Test([]byte(randomWallet.addressUncompressed)) {
			SaveWallet(randomWallet, *output_opt)
			if CheckWallet(sqliteDatabase, randomWallet.addressUncompressed) {
				SaveWallet(randomWallet, "balance_wallets.txt")
				totalBalancedAddress++
			}
			totalFound++
		}
		total++
	}
}

func checkBalance(wallet string) string {
	resp, err := http.Get(BalanceAPI + wallet)
	if err != nil {
		fmt.Println(err)
		return "0.00000000"
	}
	var generic map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&generic)
	if err != nil {
		fmt.Println(err)
		return "0.00000000"
	} else {
		if generic["data"] != nil {
			balance := fmt.Sprint(generic["data"].(map[string]interface{})["confirmed_balance"])
			if balance != "0.00000000" && balance != "<nil>" {
				totalBalancedAddress++
				return balance
			} else {
				return balance
			}
		} else {
			return "0.00000000"
		}
	}
}

func RemoveIndex(s []string, index int) []string {
	return append(s[:index], s[index+1:]...)
}

func GeneratorFull(passphrase string) Wallet {
	hasher := sha256.New() // SHA256
	sha := SHA256(hasher, []byte(passphrase))
	_, public := btcec.PrivKeyFromBytes(btcec.S256(), sha)

	// Get compressed and uncompressed addresses
	// caddr, _ := btcutil.NewAddressPubKey(public.SerializeCompressed(), &chaincfg.MainNetParams)
	uaddr, _ := btcutil.NewAddressPubKey(public.SerializeUncompressed(), &chaincfg.MainNetParams)
	if *verbose_opt {
		fmt.Println(passphrase, uaddr.EncodeAddress())
	}
	return Wallet{addressUncompressed: uaddr.EncodeAddress(), passphrase: passphrase} // Send line to output channel
	// return Wallet{addressUncompressed: uaddr.EncodeAddress(), addressCompressed: caddr.EncodeAddress(), passphrase: passphrase} // Send line to output channel
}

func AddressToRIPEM160(address string) string {
	baseBytes := base58.Decode(address)
	end := len(baseBytes) - 4
	hash := baseBytes[0:end]
	return hex.EncodeToString(hash)[2:]
}

func SaveWallet(walletInfo Wallet, path string) {
	fullWallet := GeneratorFull(walletInfo.passphrase)
	f, err := os.OpenFile(path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(fullWallet.addressUncompressed + ":" + fullWallet.passphrase + "\n"); err != nil {
		log.Println(err)
	}
}

func CheckWallet(db *sql.DB, hash string) bool {
	sqlStmt := `SELECT hash FROM address WHERE hash = ?`
	err := db.QueryRow(sqlStmt, hash).Scan(&hash)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Print(err)
		}
		return false
	}
	return true
}

func insertaddressBatch(db *sql.DB, hash string) {
	insertHash := `INSERT INTO address(hash) VALUES ` + hash
	statement, err := db.Prepare(insertHash)

	//println(insertHash)
	if err != nil {
		log.Fatalln(err.Error())
	}
	_, err = statement.Exec()
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func createDB() {
	log.Println("Creating Database...")
	file, err := os.Create("database.db") // Create SQLite file
	if err != nil {
		log.Fatal(err.Error())
	}
	file.Close()
	log.Println("DB created")
}

func InsertWalletToDB() {
	createTable(sqliteDatabase) // Create Database Tables
	var tempCodes []string
	var currentIndex int = 0
	for _, address := range addressList {
		tempCodes = append(tempCodes, address) //tempCodes = append(tempCodes, AddressToRIPEM160(address))
		if currentIndex == 10000 {
			code := `("` + strings.Join(tempCodes, `") , ("`) + `")`
			insertaddressBatch(sqliteDatabase, code)
			tempCodes = nil
			currentIndex = 0
		}
		currentIndex++
	}
	if len(tempCodes) <= 10000 {
		insertaddressBatch(sqliteDatabase, `("`+strings.Join(tempCodes, `") , ("`)+`")`)
		tempCodes = nil
	}
	fmt.Println("Wallets Inserted Databased.")
}

func createTable(db *sql.DB) {
	createHashTable := `CREATE TABLE "address" (
		"id"	INTEGER UNIQUE,
		"hash"	TEXT NOT NULL UNIQUE,
		PRIMARY KEY("id" AUTOINCREMENT)
	);` // SQL Statement for Create Table
	log.Println("Creating table...")
	statement, err := db.Prepare(createHashTable) // Prepare SQL Statement
	if err != nil {
		log.Fatal(err.Error())
	}
	statement.Exec() // Execute SQL Statements
	log.Println("Table created")
}

func insertaddress(db *sql.DB, hash string) {
	insertHash := `INSERT INTO address(hash) VALUES (?)`
	statement, err := db.Prepare(insertHash)
	if err != nil {
		log.Fatalln(err.Error())
	}
	_, err = statement.Exec(hash)
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func displayHashs(db *sql.DB) {
	row, err := db.Query("SELECT * FROM address")
	if err != nil {
		log.Fatal(err)
	}
	defer row.Close()
	for row.Next() {
		var hashwallet string
		row.Scan(&hashwallet)
		//log.Println("Wallet: ", hashwallet)
	}
}

func RandomPhrase(length int) string {
	var letters [58]string = [58]string{"A", "B", "C", "D", "E", "F", "G", "H", "J", "K", "L", "M", "N", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "2", "3", "4", "5", "6", "7", "8", "9"}
	serial := letters[rand.Intn(len(letters))]
	for i := 1; i < length; i++ {
		serial += letters[rand.Intn(len(letters))]
	}
	return "S" + serial
}

func randInt(min int, max int) int {
	return min + rand.Intn(max-min)
}

// SHA256 Hasher function
func SHA256(hasher hash.Hash, input []byte) (hash []byte) {

	hasher.Reset()
	hasher.Write(input)
	hash = hasher.Sum(nil)
	return hash

}

// RIPEMD160 Hasher function
func RIPEMD160(input []byte) (hash []byte) {

	riper := ripemd160.New()
	riper.Write(input)
	hash = riper.Sum(nil)
	return hash

}

func appendWordlist(a []string, b []string) []string {
	check := make(map[string]int)
	d := append(a, b...)
	res := make([]string, 0)
	for _, val := range d {
		check[val] = 1
	}
	for letter, _ := range check {
		res = append(res, letter)
	}
	return res
}
