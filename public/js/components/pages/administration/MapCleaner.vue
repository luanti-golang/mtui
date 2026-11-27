<script>
import DefaultLayout from "../../layouts/DefaultLayout.vue";
import { START, ADMINISTRATION, MAPCLEANER } from "../../../util/breadcrumbs.js";
import { get_settings, set_settings, get_status, start, pause, reset } from "../../../api/mapcleaner.js";
import { confirm } from "../../../store/confirm.js";
import { success } from "../../../store/toast.js";
import format_time from "../../../util/format_time.js";

// nodes per chunk edge (5 mapblocks * 16 nodes)
const CHUNK_NODES = 80;

const AXES = ["x", "y", "z"];

export default {
    components: {
        "default-layout": DefaultLayout
    },
    data: function() {
        return {
            breadcrumb: [START, ADMINISTRATION, MAPCLEANER],
            axes: AXES,
            settings: null,
            protected_nodes_text: "",
            status: null,
            busy: false,
            dirty: false,
            rate: null,
            last_sample: null,
            timer: null
        };
    },
    mounted: async function() {
        await this.load_settings();
        await this.update_status();
        this.timer = setInterval(() => this.update_status(), 2000);
    },
    unmounted: function() {
        clearInterval(this.timer);
    },
    computed: {
        running: function() {
            return this.status && this.status.status == "running";
        },
        total: function() {
            const s = this.settings;
            if (!s) {
                return 0;
            }
            return (s.to_x - s.from_x + 1) * (s.to_y - s.from_y + 1) * (s.to_z - s.from_z + 1);
        },
        progress: function() {
            if (!this.status || !this.status.total_chunks) {
                return 0;
            }
            return Math.min(100, this.status.processed / this.status.total_chunks * 100);
        },
        eta: function() {
            if (!this.running || !this.rate) {
                return null;
            }
            const remaining = this.status.total_chunks - this.status.processed;
            return Math.round(remaining / this.rate);
        },
        status_class: function() {
            return {
                idle: "text-bg-secondary",
                running: "text-bg-primary",
                paused: "text-bg-warning",
                done: "text-bg-success",
                error: "text-bg-danger"
            }[this.status && this.status.status] || "text-bg-secondary";
        },
        resumable: function() {
            return this.status && (this.status.status == "paused" || this.status.status == "error") &&
                this.settings && this.status.dry_run == this.settings.dry_run;
        }
    },
    methods: {
        format_time: format_time,
        node_range: function(axis) {
            const from = this.settings["from_" + axis] * CHUNK_NODES;
            const to = this.settings["to_" + axis] * CHUNK_NODES + CHUNK_NODES - 1;
            return `${from} .. ${to}`;
        },
        format_duration: function(seconds) {
            const h = Math.floor(seconds / 3600);
            const m = Math.floor(seconds % 3600 / 60);
            const s = seconds % 60;
            return (h > 0 ? `${h}h ` : "") + (h > 0 || m > 0 ? `${m}m ` : "") + `${s}s`;
        },
        apply_settings: function(s) {
            this.settings = s;
            this.protected_nodes_text = (s.protected_nodes || []).join("\n");
            this.dirty = false;
        },
        load_settings: async function() {
            this.apply_settings(await get_settings());
        },
        update_status: async function() {
            const status = await get_status();
            const now = Date.now();
            if (status.status == "running" && this.last_sample && status.processed > this.last_sample.processed) {
                const rate = (status.processed - this.last_sample.processed) / ((now - this.last_sample.time) / 1000);
                // smooth the rate a bit
                this.rate = this.rate ? this.rate * 0.7 + rate * 0.3 : rate;
            }
            if (status.status != "running") {
                this.rate = null;
            }
            this.last_sample = { processed: status.processed, time: now };
            this.status = status;
        },
        save: async function() {
            this.busy = true;
            try {
                const s = Object.assign({}, this.settings, {
                    protected_nodes: this.protected_nodes_text.split("\n")
                });
                for (const key of ["neighbor_range", "delay_ms", "from_x", "from_y", "from_z", "to_x", "to_y", "to_z"]) {
                    s[key] = parseInt(s[key]) || 0;
                }
                this.apply_settings(await set_settings(s));
                success("Map cleanup settings saved");
                await this.update_status();
            } finally {
                this.busy = false;
            }
        },
        start: async function() {
            if (!this.settings.dry_run) {
                const ok = await confirm({
                    title: this.resumable ? "Resume map cleanup?" : "Start map cleanup?",
                    message: "Map chunks without protected nodes, outside of protected areas and away from protected chunks are permanently removed from the map database and will be generated anew when visited. Make sure a backup of the map exists.",
                    confirm_label: this.resumable ? "Resume removal" : "Start removal"
                });
                if (!ok) {
                    return;
                }
            }
            this.busy = true;
            try {
                this.status = await start();
                this.last_sample = null;
            } finally {
                this.busy = false;
            }
        },
        pause: async function() {
            this.busy = true;
            try {
                this.status = await pause();
            } finally {
                this.busy = false;
            }
        },
        reset: async function() {
            const ok = await confirm({
                title: "Reset progress?",
                message: "The position and counters are reset, the next run starts at the beginning of the range. Already removed chunks are not restored.",
                confirm_label: "Reset",
                danger: false
            });
            if (!ok) {
                return;
            }
            this.busy = true;
            try {
                this.status = await reset();
            } finally {
                this.busy = false;
            }
        }
    }
};
</script>

<template>
  <default-layout icon="broom" title="Map cleanup" :breadcrumb="breadcrumb">
      <div class="alert alert-warning">
          <i class="fa-solid fa-triangle-exclamation"></i>
          Removes map chunks that contain none of the protected nodes and aren't covered by an area protection.
          Chunks next to protected chunks are kept as a safety margin. Removed chunks are generated anew by the mapgen.
          <b>Make a backup of the map database before running this without dry-run.</b>
      </div>

      <div v-if="!settings || !status">
          <i class="fa-solid fa-spinner fa-spin"></i>
          Loading
      </div>
      <div class="row" v-else>
          <div class="col-lg-5 mb-3">
              <div class="card">
                  <div class="card-header d-flex align-items-center">
                      <span class="me-auto">Status</span>
                      <span class="badge" :class="status_class">{{status.status}}</span>
                      <span class="badge text-bg-info ms-1" v-if="status.dry_run">dry-run</span>
                  </div>
                  <div class="card-body">
                      <div class="progress mb-2" role="progressbar" :aria-valuenow="progress" aria-valuemin="0" aria-valuemax="100">
                          <div class="progress-bar" :class="{'progress-bar-striped progress-bar-animated': running}" :style="{width: progress + '%'}">
                              {{progress.toFixed(1)}}%
                          </div>
                      </div>
                      <table class="table table-sm mb-2">
                          <tbody>
                              <tr>
                                  <td>Processed chunks</td>
                                  <td>{{status.processed}} / {{status.total_chunks}}</td>
                              </tr>
                              <tr>
                                  <td>{{status.dry_run ? "Would be removed" : "Removed chunks"}}</td>
                                  <td>{{status.removed}}</td>
                              </tr>
                              <tr>
                                  <td>Retained chunks</td>
                                  <td>{{status.retained}}</td>
                              </tr>
                              <tr>
                                  <td>Current chunk</td>
                                  <td>{{status.chunk_x}}, {{status.chunk_y}}, {{status.chunk_z}}</td>
                              </tr>
                              <tr v-if="rate">
                                  <td>Speed</td>
                                  <td>{{rate.toFixed(1)}} chunks/s</td>
                              </tr>
                              <tr v-if="eta != null">
                                  <td>Remaining</td>
                                  <td>~{{format_duration(eta)}}</td>
                              </tr>
                              <tr v-if="status.started_at">
                                  <td>Started</td>
                                  <td>{{format_time(status.started_at / 1000)}}</td>
                              </tr>
                          </tbody>
                      </table>
                      <div class="alert alert-danger" v-if="status.error">
                          {{status.error}}
                      </div>
                      <div class="btn-group">
                          <button class="btn" :class="settings.dry_run ? 'btn-primary' : 'btn-danger'" v-if="!running" :disabled="busy || dirty" v-on:click="start">
                              <i class="fa-solid fa-play"></i>
                              {{resumable ? "Resume" : "Start"}}{{settings.dry_run ? " dry-run" : ""}}
                          </button>
                          <button class="btn btn-warning" v-if="running" :disabled="busy" v-on:click="pause">
                              <i class="fa-solid fa-pause"></i>
                              Pause
                          </button>
                          <button class="btn btn-outline-secondary" :disabled="busy || running" v-on:click="reset">
                              <i class="fa-solid fa-rotate-left"></i>
                              Reset
                          </button>
                      </div>
                      <div class="form-text" v-if="dirty">Save the settings before starting</div>
                  </div>
              </div>
          </div>

          <div class="col-lg-7">
              <div class="card">
                  <div class="card-header">Settings</div>
                  <fieldset class="card-body" :disabled="running" v-on:input="dirty = true" v-on:change="dirty = true">
                      <div class="mb-3">
                          <label class="form-label" for="mc-nodes">Protected node names (one per line)</label>
                          <textarea id="mc-nodes" class="form-control font-monospace" rows="6" v-model="protected_nodes_text"
                              placeholder="bones:bones&#10;travelnet:travelnet&#10;protector:protect"></textarea>
                          <div class="form-text">Chunks containing at least one of these nodes are kept, lines starting with # are ignored</div>
                      </div>
                      <div class="form-check mb-3">
                          <input class="form-check-input" type="checkbox" id="mc-areas" v-model="settings.use_areas"/>
                          <label class="form-check-label" for="mc-areas">
                              Keep chunks inside protected areas (<code>areas.dat</code> / <code>areas.json</code> from the areas mod)
                          </label>
                      </div>
                      <div class="row mb-3">
                          <div class="col-sm-6">
                              <label class="form-label" for="mc-neighbor">Safety margin (chunks)</label>
                              <input id="mc-neighbor" class="form-control" type="number" min="0" max="5" v-model="settings.neighbor_range"/>
                              <div class="form-text">Neighboring chunks of protected chunks to keep</div>
                          </div>
                          <div class="col-sm-6">
                              <label class="form-label" for="mc-delay">Delay per chunk (ms)</label>
                              <input id="mc-delay" class="form-control" type="number" min="0" v-model="settings.delay_ms"/>
                              <div class="form-text">Throttles the database load</div>
                          </div>
                      </div>
                      <label class="form-label">Range in chunk coordinates (1 chunk = 80 nodes)</label>
                      <table class="table table-sm align-middle">
                          <thead>
                              <tr>
                                  <th></th>
                                  <th>From</th>
                                  <th>To</th>
                                  <th>Nodes</th>
                              </tr>
                          </thead>
                          <tbody>
                              <tr v-for="axis in axes" :key="axis">
                                  <th>{{axis.toUpperCase()}}</th>
                                  <td><input class="form-control form-control-sm" type="number" min="-400" max="400" v-model="settings['from_' + axis]"/></td>
                                  <td><input class="form-control form-control-sm" type="number" min="-400" max="400" v-model="settings['to_' + axis]"/></td>
                                  <td class="small text-body-secondary text-nowrap">{{node_range(axis)}}</td>
                              </tr>
                          </tbody>
                      </table>
                      <div class="form-text mb-3">{{total}} chunks in range, changing the range resets the progress</div>
                      <div class="form-check mb-3">
                          <input class="form-check-input" type="checkbox" id="mc-dryrun" v-model="settings.dry_run"/>
                          <label class="form-check-label" for="mc-dryrun">
                              Dry-run (only count the chunks that would be removed)
                          </label>
                      </div>
                      <button class="btn btn-primary" :disabled="busy" v-on:click="save">
                          <i class="fa-solid fa-floppy-disk"></i>
                          Save
                      </button>
                  </fieldset>
              </div>
          </div>
      </div>
  </default-layout>
</template>
