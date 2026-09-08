package service

import "testing"

func TestVerifiedAccountKeepsOnlyItsOwnDirectory(t *testing.T) {
	library, _, payload := libraryFixture(t)
	drive := &PanService{database: library.database, directory: panLibraryDirectory{
		AccountID: payload.Source.AccountID, PanLibraryDirectory: payload.Source.Directory,
	}}
	if err := drive.discardOtherAccountDirectory(t.Context(), payload.Source.AccountID); err != nil {
		t.Fatal(err)
	}
	source, err := loadLibrarySource(t.Context(), library.database)
	if err != nil || source == nil || source.Directory.ID != payload.Source.Directory.ID {
		t.Fatalf("same-account selection: source=%v err=%v", source, err)
	}
	if err := drive.discardOtherAccountDirectory(t.Context(), "different-account"); err != nil {
		t.Fatal(err)
	}
	source, err = loadLibrarySource(t.Context(), library.database)
	if err != nil || source != nil || drive.directory.ID != "" || drive.authorizationVersion != 1 {
		t.Fatalf("old account selection retained: source=%v drive=%+v err=%v", source, drive.directory, err)
	}
}
