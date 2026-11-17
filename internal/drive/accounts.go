package drive

import (
	"context"
	"sort"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/pan"
)

const accountsSetting = "pan.accounts"

// accountRecord keeps one 115 account's tokens and mounted directory so several
// accounts can be listed and, in later steps, scanned independently.
type accountRecord struct {
	Tokens    pan.Tokens  `json:"tokens"`
	Directory mountRecord `json:"directory"`
}

type accountStore map[string]accountRecord

func (d *Drive) loadAccounts(ctx context.Context) (accountStore, error) {
	store, _, err := database.LoadSetting[accountStore](ctx, d.database, accountsSetting)
	if err != nil {
		return nil, err
	}
	if store == nil {
		store = accountStore{}
	}
	return store, nil
}

func (d *Drive) saveAccountTokens(ctx context.Context, accountID string, tokens pan.Tokens) error {
	if accountID == "" {
		return nil
	}
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return err
	}
	record := store[accountID]
	record.Tokens = tokens
	store[accountID] = record
	d.setAccountToken(accountID, tokens)
	return database.SaveSetting(ctx, d.database, accountsSetting, store)
}

func (d *Drive) accountToken(accountID string) pan.Tokens {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.accountTokens[accountID]
}

func (d *Drive) setAccountToken(accountID string, tokens pan.Tokens) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.accountTokens == nil {
		d.accountTokens = make(map[string]pan.Tokens)
	}
	d.accountTokens[accountID] = tokens
}

func (d *Drive) saveAccountMount(ctx context.Context, accountID string, mount mountRecord) error {
	if accountID == "" {
		return nil
	}
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return err
	}
	record := store[accountID]
	record.Directory = mount
	store[accountID] = record
	return database.SaveSetting(ctx, d.database, accountsSetting, store)
}

func (d *Drive) removeAccount(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return err
	}
	if _, ok := store[accountID]; !ok {
		return nil
	}
	delete(store, accountID)
	d.setAccountToken(accountID, pan.Tokens{})
	return database.SaveSetting(ctx, d.database, accountsSetting, store)
}

func (d *Drive) currentAccountID() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.accountID
}

func (d *Drive) accountRecord(ctx context.Context, accountID string) (accountRecord, error) {
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return accountRecord{}, err
	}
	return store[accountID], nil
}

// Accounts lists every stored 115 account and its mounted directory.
func (d *Drive) Accounts(ctx context.Context) ([]AccountStatus, error) {
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(store))
	for id := range store {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]AccountStatus, 0, len(ids))
	for _, id := range ids {
		record := store[id]
		account, err := d.client.Account(ctx, record.Tokens.AccessToken)
		if err != nil {
			result = append(result, AccountStatus{Connected: false})
			continue
		}
		status := AccountStatus{Connected: true, Account: &account}
		if record.Directory.ID != "" && record.Directory.AccountID == account.ID {
			directory := record.Directory.LibraryDirectory
			status.Directory = &directory
		}
		result = append(result, status)
	}
	return result, nil
}
