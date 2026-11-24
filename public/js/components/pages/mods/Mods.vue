<script>
import {
	add,
	get_mods_by_type,
	get_git_mod,
	check_updates,
	add_mtui,
	update,
	get_all
} from '../../../service/mods.js';
import { get_vcs_packages } from '../../../api/mods.js';
import { update_settings } from '../../../service/mtconfig.js';

import FeedbackButton from '../../FeedbackButton.vue';
import DefaultLayout from '../../layouts/DefaultLayout.vue';
import { START, ADMINISTRATION, MODS } from '../../../util/breadcrumbs.js';
import { has_feature } from '../../../service/features.js';
import ModRow from "./ModRow.vue";
import { success } from '../../../store/toast.js';

const modname_regex = /^[a-zA-Z0-9_]*$/;

export default {
	components: {
		"feedback-button": FeedbackButton,
		"default-layout": DefaultLayout,
		"mod-row": ModRow
	},
	data: () => {
		return {
			add_name: "",
			add_mod_type: "mod",
			add_source_type: "git",
			add_url: "",
			add_version: "",
			breadcrumb: [START, ADMINISTRATION, MODS],
			update_handle: null,
			busy: false,
			vcs_packages: []
		};
	},
	mounted: async function() {
		try {
			const list = await get_vcs_packages();
			this.vcs_packages = (list || []).filter(p => p.repo && p.name);
		} catch (e) {
			// autocomplete is optional
		}
	},
	watch: {
		add_name: function(name) {
			if (!name || this.add_url) {
				return;
			}
			const pkg = this.vcs_packages.find(p => p.name == name);
			if (!pkg) {
				return;
			}
			this.add_url = pkg.repo;
			if (["mod", "game", "txp"].includes(pkg.type)) {
				this.add_mod_type = pkg.type;
			}
		}
	},
	created: function() {
        this.update_handle = setInterval(update, 2000);
    },
    unmounted: function() {
        clearInterval(this.update_handle);
    },
	methods: {
		add: async function() {
			await add({
				name: this.add_name,
				mod_type: this.add_mod_type,
				source_type: this.add_source_type,
				url: this.add_url,
				branch: "",
				version: this.add_version
			});
			success(`Added ${this.add_name}`);
			this.add_name = "";
			this.add_url = "";
			this.add_version = "";
		},
		add_mtui_mod: async function() {
			await add_mtui();
			if (has_feature("minetest_config")) {
				await update_settings();
			}
			success("Installed the mtui mod");
		},
		get_git_mod,
		check_updates: async function() {
			this.busy = true;
			try {
				await check_updates();
				const count = get_all().filter(m => m.latest_version && m.latest_version != m.version).length;
				success(count > 0 ? `${count} update(s) available` : "All mods are up to date");
			} finally {
				this.busy = false;
			}
		}
	},
	computed: {
		games: () => get_mods_by_type("game"),
		mods: () => get_mods_by_type("mod"),
		worldmods: () => get_mods_by_type("worldmods"),
		txps: () => get_mods_by_type("txp"),
		name_suggestions: function() {
			const q = this.add_name.toLowerCase();
			if (!q) {
				return [];
			}
			const prefix = [];
			const contains = [];
			for (const p of this.vcs_packages) {
				const n = p.name.toLowerCase();
				if (n.startsWith(q)) {
					prefix.push(p);
				} else if (n.includes(q)) {
					contains.push(p);
				}
			}
			return prefix.concat(contains).slice(0, 50);
		},
		add_name_valid: function() {
			return (this.add_name == "" || modname_regex.test(this.add_name));
		}
	}
};
</script>

<template>
  <default-layout icon="cubes" title="Mods" :breadcrumb="breadcrumb">
  	<h4>
  		Mod management
  	</h4>
  	<div class="alert alert-warning" v-if="!get_git_mod('mtui')">
  		<div class="row">
  			<div class="col-12">
  				<i class="fa-solid fa-triangle-exclamation"></i>
  				<b>Warning:</b>
  				The <i>mtui</i> mod is not installed, some features may not work properly
  				<button class="btn btn-primary float-end" :disabled="busy" v-on:click="add_mtui_mod">
  					<i class="fa fa-plus"></i>
  					Install the "mtui" mod
  					<i class="fa-solid fa-spinner fa-spin" v-if="busy"></i>
  				</button>
  			</div>
  		</div>
  	</div>
  	<div class="row">
  		<div class="col-md-10"></div>
  		<div class="col-md-2">
  			<button class="btn btn-primary w-100" v-on:click="check_updates" :disabled="busy">
  				<i class="fa fa-refresh" v-bind:class="{'fa-spin': busy}"></i>
  				Check for updates
  			</button>
  		</div>
  	</div>
  	<table class="table table-condensed table-striped">
  		<thead>
  			<tr>
  				<th>Type</th>
  				<th>Name</th>
  				<th>Source</th>
  				<th>Version</th>
  				<th>Auto-update</th>
  				<th>Actions</th>
  			</tr>
  		</thead>
  		<tbody>
  			<tr>
  				<td>
  					<select class="form-control" v-model="add_mod_type">
  						<option value="mod">Mod</option>
  						<option value="worldmods">Worldmods</option>
  						<option value="game">Game</option>
  						<option value="txp">Texturepack</option>
  					</select>
  				</td>
  				<td>
  					<div class="input-group has-validation">
  						<input
  							class="form-control"
  							type="text"
  							placeholder="Mod name"
  							list="vcs-packages"
  							autocomplete="off"
  							v-model="add_name"
  							v-bind:class="{'is-invalid': !add_name_valid}"/>
  						<datalist id="vcs-packages">
  							<option v-for="p in name_suggestions" :key="p.author + '/' + p.name" :value="p.name">
  								{{p.title}} ({{p.author}})
  							</option>
  						</datalist>
  						<div class="invalid-feedback" v-if="!add_name_valid">
  							Allowed modname characters: a-z A-Z 0-9 _
  						</div>
  					</div>
  				</td>
  				<td>
  					<input class="form-control" type="text" placeholder="Source url" v-model="add_url"/>
  				</td>
  				<td>
  					<input class="form-control" type="text" placeholder="Version" v-model="add_version"/>
  				</td>
  				<td></td>
  				<td>
  					<feedback-button type="success" :fn="add" :disabled="!add_name || !add_url">
  						<i class="fa-brands fa-git-alt"></i>
  						Add from git
  					</feedback-button>
  				</td>
  			</tr>
  			<tr v-if="games.length > 0" class="table-secondary">
  				<td colspan="8">
  					<h4>Game</h4>
  				</td>
  			</tr>
  			<tr v-for="mod in games" :key="mod.id">
  				<mod-row :mod="mod"/>
  			</tr>
  			<tr v-if="txps.length > 0" class="table-secondary">
  				<td colspan="8">
  					<h4>Texture-packs</h4>
  				</td>
  			</tr>
  			<tr v-for="mod in txps" :key="mod.id">
  				<mod-row :mod="mod"/>
  			</tr>
  			<tr v-if="mods.length > 0" class="table-secondary">
  				<td colspan="8">
  					<h4>Mods</h4>
  				</td>
  			</tr>
  			<tr v-for="mod in mods" :key="mod.id">
  				<mod-row :mod="mod"/>
  			</tr>
  			<tr v-if="worldmods.length > 0" class="table-secondary">
  				<td colspan="8">
  					<h4>Worldmods collection</h4>
  				</td>
  			</tr>
  			<tr v-for="worldmod in worldmods" :key="worldmod.id">
  				<mod-row :mod="worldmod"/>
  			</tr>
  		</tbody>
  	</table>
  </default-layout>
</template>
