-- Migration 002: Add confidence level to applications

ALTER TABLE applications ADD COLUMN confidence INTEGER;
