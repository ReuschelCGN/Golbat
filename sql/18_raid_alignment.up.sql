ALTER TABLE `gym`
    ADD COLUMN IF NOT EXISTS `raid_pokemon_alignment` smallint unsigned DEFAULT NULL;
