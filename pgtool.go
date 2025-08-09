package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: pgtool <backup|restore> [options]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "backup":
		backupCmd := flag.NewFlagSet("backup", flag.ExitOnError)
		dbName := backupCmd.String("db", "", "Database name (required)")
		dbUser := backupCmd.String("user", "postgres", "PostgreSQL user")
		dbHost := backupCmd.String("host", "localhost", "PostgreSQL host")
		backupDir := backupCmd.String("backup-dir", "/var/backups/postgresql", "Backup directory")
		logFile := backupCmd.String("log-file", "/var/log/postgres_backup.log", "Log file path")
		retentionDays := backupCmd.Int("retention", 7, "Retention period in days")

		backupCmd.Parse(os.Args[2:])
		runBackup(*dbName, *dbUser, *dbHost, *backupDir, *logFile, *retentionDays)

	case "restore":
		restoreCmd := flag.NewFlagSet("restore", flag.ExitOnError)
		dbName := restoreCmd.String("db", "", "Database name (required)")
		dbUser := restoreCmd.String("user", "postgres", "PostgreSQL user")
		dbHost := restoreCmd.String("host", "localhost", "PostgreSQL host")
		backupFile := restoreCmd.String("file", "", "Backup file (.dump.gz) to restore (required)")
		logFile := restoreCmd.String("log-file", "/var/log/postgres_backup.log", "Log file path")

		restoreCmd.Parse(os.Args[2:])
		runRestore(*dbName, *dbUser, *dbHost, *backupFile, *logFile)

	default:
		fmt.Println("Unknown command:", os.Args[1])
		fmt.Println("Usage: pgtool <backup|restore> [options]")
		os.Exit(1)
	}
}

func runBackup(dbName, dbUser, dbHost, backupDir, logFile string, retentionDays int) {
	if dbName == "" {
		fmt.Println("Error: Database name is required.")
		os.Exit(1)
	}

	// Ensure backup directory exists
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		fmt.Printf("Error: Backup directory '%s' not found.\n", backupDir)
		os.Exit(1)
	}

	// Open log file
	logF, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error: Cannot open log file '%s': %v\n", logFile, err)
		os.Exit(1)
	}
	defer logF.Close()
	logger := log.New(logF, "", log.LstdFlags)

	// Create backup filename
	timestamp := time.Now().Format("2006-01-02_150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s_%s.dump", dbName, timestamp))

	// Run pg_dump
	logger.Printf("INFO: Starting backup for database '%s'.", dbName)
	fmt.Printf("Starting backup for database '%s'...\n", dbName)

	cmd := exec.Command(
		"pg_dump",
		"-U", dbUser,
		"-h", dbHost,
		"-Fc", dbName,
	)
	outFile, err := os.Create(backupFile)
	if err != nil {
		logger.Printf("ERROR: Cannot create backup file: %v", err)
		fmt.Println("Backup failed.")
		os.Exit(1)
	}
	defer outFile.Close()
	cmd.Stdout = outFile
	cmd.Stderr = logF

	// Pass password from env if set
	if pw := os.Getenv("PGPASSWORD"); pw != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pw))
	}

	if err := cmd.Run(); err != nil {
		logger.Printf("ERROR: Backup failed: %v", err)
		fmt.Println("Backup failed. Check log for details.")
		os.Remove(backupFile)
		os.Exit(1)
	}

	// Compress backup
	compressedFile := backupFile + ".gz"
	if err := compressFile(backupFile, compressedFile); err != nil {
		logger.Printf("ERROR: Compression failed: %v", err)
		fmt.Println("Compression failed.")
		os.Exit(1)
	}
	os.Remove(backupFile)

	logger.Printf("SUCCESS: Backup completed. File: %s", compressedFile)
	fmt.Println("Backup successful:", compressedFile)

	// Cleanup old backups
	cleanupOldBackups(backupDir, retentionDays, logger)
}

func runRestore(dbName, dbUser, dbHost, backupFile, logFile string) {
	if dbName == "" || backupFile == "" {
		fmt.Println("Error: Database name and backup file are required.")
		os.Exit(1)
	}

	// Open log file
	logF, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error: Cannot open log file '%s': %v\n", logFile, err)
		os.Exit(1)
	}
	defer logF.Close()
	logger := log.New(logF, "", log.LstdFlags)

	logger.Printf("INFO: Starting restore for database '%s' from '%s'.", dbName, backupFile)
	fmt.Printf("Restoring database '%s' from '%s'...\n", dbName, backupFile)

	// Decompress to temp file
	tempFile := backupFile[:len(backupFile)-3] // remove .gz
	if err := decompressFile(backupFile, tempFile); err != nil {
		logger.Printf("ERROR: Decompression failed: %v", err)
		fmt.Println("Decompression failed.")
		os.Exit(1)
	}
	defer os.Remove(tempFile)

	// Run pg_restore
	cmd := exec.Command(
		"pg_restore",
		"-U", dbUser,
		"-h", dbHost,
		"-d", dbName,
		"--clean", // drop objects before recreating
		tempFile,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = logF

	if pw := os.Getenv("PGPASSWORD"); pw != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", pw))
	}

	if err := cmd.Run(); err != nil {
		logger.Printf("ERROR: Restore failed: %v", err)
		fmt.Println("Restore failed. Check log for details.")
		os.Exit(1)
	}

	logger.Printf("SUCCESS: Restore completed for database '%s'.", dbName)
	fmt.Println("Restore completed successfully.")
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	_, err = io.Copy(gw, in)
	return err
}

func decompressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	gr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gr.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, gr)
	return err
}

func cleanupOldBackups(backupDir string, retentionDays int, logger *log.Logger) {
	logger.Printf("INFO: Cleaning up backups older than %d days.", retentionDays)
	fmt.Println("Cleaning up old backups...")

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".gz" {
			if info.ModTime().Before(cutoff) {
				if rmErr := os.Remove(path); rmErr == nil {
					logger.Printf("INFO: Deleted old backup: %s", path)
				} else {
					logger.Printf("WARNING: Failed to delete %s: %v", path, rmErr)
				}
			}
		}
		return nil
	})
	logger.Println("SUCCESS: Cleanup complete.")
	fmt.Println("Cleanup complete.")
}