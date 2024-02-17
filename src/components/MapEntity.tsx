import classNames from "classnames";
import * as maptalks from "maptalks";
import ms from "milsymbol";
import React, { useEffect, useMemo, useRef, useState } from "react";
import { BiX, BiExit, BiMapPin } from "react-icons/bi";
import { DCSMap } from "../dcs/maps/DCSMap";
import { useKeyPress } from "../hooks/useKeyPress";
import { Alert, alertStore } from "../stores/AlertStore";
import {
  popEntityTag,
  pushEntityTag,
  useEntityMetadata,
} from "../stores/EntityMetadataStore";
import { serverStore, setSelectedEntityId } from "../stores/ServerStore";
import {
  EntityTrackPing,
  estimatedSpeed,
  setTrackOptions,
  trackStore,
} from "../stores/TrackStore";
import { Entity } from "../types/entity";
import { getBearingMap, getCardinal, getFlyDistance } from "../util";
import DetailedCoords from "./DetailedCoords";
import { colorMode } from "./MapIcon";

export const iconCache: Record<string, string> = {};

export function EntityInfo({
  map,
  dcsMap,
  entity,
  track,
}: {
  map: maptalks.Map;
  dcsMap: DCSMap;
  entity: Entity;
  track: Array<EntityTrackPing> | null;
}) {
  const trackOptions = trackStore((state) => state.trackOptions.get(entity.id));
  const alerts = alertStore((state) => state.alerts.get(entity.id));
  const entities = serverStore((state) => state.entities);
  const metadata = useEntityMetadata(entity.id);
  const [addTagText, setAddTagText] = useState("");
  const inputRef = useRef(null);
  const isEnterPressed = useKeyPress("Enter");

	//console.log("entity");
	//console.log(entity);
	//console.log("track");
	//console.log(track);

  useEffect(() => {
    if (!inputRef.current) return;
    if (
      isEnterPressed &&
      addTagText !== "" &&
      document.activeElement === inputRef.current
    ) {
      pushEntityTag(entity.id, addTagText);
      setAddTagText("");
    }
  }, [isEnterPressed, inputRef, addTagText]);

  let alertEntities = useMemo(
    () =>
      alerts
        ?.map((it) => {
          const targetEntity = entities.get(it.targetEntityId);
          if (!targetEntity) {
            return;
          }

          const distance = getFlyDistance(
            [entity.latitude, entity.longitude],
            [targetEntity.latitude, targetEntity.longitude]
          );

          return [it, targetEntity, distance];
        })
        .filter((it): it is [Alert, Entity, number] => it !== undefined)
        .sort(([x, y, a], [e, f, b]) => a - b),
    [alerts, entities]
  );
//console.log("testouille");
  return (
    <div className="flex flex-col bg-gray-300 border border-gray-500 shadow select-none rounded-sm">
      <div className="p-2 bg-gray-400 text-sm flex flex-row gap-2">
        <b>{entity.name}</b>
		<button
		  className="p-1 text-xs bg-blue-300 border border-blue-400 ml-auto"
		  onClick={() => {
			map.animateTo(
			  {
				center: [entity.longitude, entity.latitude],
				zoom: 10,
			  },
			  {
				duration: 250,
				easing: "out",
			  }
			);
		  }}
		>
		  <BiMapPin className="inline-block w-4 h-4" />
		</button>
        <button
          className="p-1 text-xs bg-red-300 border border-red-400"
          onClick={() => {
            setSelectedEntityId(null);
          }}
        >
          <BiExit className="inline-block w-4 h-4" />
        </button>
      </div>
      <div className="p-2 flex flex-row">
        <div className="flex flex-col pr-2">
          {track && entity.types.includes("Air") && (
            <>
              <div>{entity.pilot}</div>
              <div>
                Heading:{" "}
                {Math.round(entity.heading).toString().padStart(3, "0")}
                {getCardinal(entity.heading)}
              </div>
              <div>GS: {Math.max(Math.round(estimatedSpeed(track)),0)}</div>
            </>
          )}
		  <div>Altitude: {Math.max(Math.round(entity.altitude * 3.28084),0)}</div>
          <div>ID: {entity.id}</div>
        </div>
      </div>
      <div className="p-2">
        <DetailedCoords coords={[entity.latitude, entity.longitude]} />
      </div>
      {metadata && (
        <div className="flex flex-col p-2">
          <div className="flex flex-row gap-2">
            {metadata.tags.map((it) => (
              <div
                className="p-1 bg-blue-200 hover:bg-blue-300 border-blue-400 border rounded-sm flex flex-row items-center"
                key={it}
              >
                <div>{it}</div>
                <button
                  onClick={() => popEntityTag(entity.id, it)}
                  className="text-red-500"
                >
                  <BiX className="inline-flex h-5 w-5 ml-1" />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
      {alertEntities && (
        <div className="flex flex-col gap-1 p-2">
          {alertEntities.map(([alert, threatEntity, distance]) => {
            const bearing = getBearingMap(
              [entity.latitude, entity.longitude],
              [threatEntity.latitude, threatEntity.longitude],
              dcsMap
            );

            return (
              <button
                className={classNames(
                  "p-1 border grid grid-cols-4 bg-gray-50",
                  {
                    "border-red-400": alert.type === "threat",
                    "border-yellow-400": alert.type === "warning",
                  }
                )}
                key={`${alert.type}-${alert.targetEntityId}`}
                onClick={() => {
                  map.animateTo(
                    {
                      center: [threatEntity.longitude, threatEntity.latitude],
                      zoom: 10,
                    },
                    {
                      duration: 250,
                      easing: "out",
                    }
                  );
                }}
              >
                <div>{threatEntity.name}</div>
                <div>
                  {bearing} {getCardinal(bearing)}
                </div>
                <div>{Math.round(distance)}NM</div>
                <div>
                  {Math.floor((threatEntity.altitude * 3.28084) / 1000)}
                </div>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function MapSimpleEntity({
  map,
  entity,
  size,
  strokeWidth,
}: {
  map: maptalks.Map;
  entity: Entity;
  size?: number;
  strokeWidth?: number;
}) {
  useEffect(() => {
    const iconLayer = map.getLayer("track-icons") as maptalks.VectorLayer;
    let marker = iconLayer.getGeometryById(entity.id) as maptalks.Marker;
    if (!marker) {
      if (iconCache[entity.sidc] === undefined) {
        iconCache[entity.sidc] = new ms.Symbol(entity.sidc, {
          size: size || 16,
          frame: true,
          fill: false,
          colorMode: colorMode,
          strokeWidth: strokeWidth || 8,
        }).toDataURL();
      }
      marker = new maptalks.Marker([entity.longitude, entity.latitude], {
        id: entity.id,
        draggable: false,
        visible: true,
        editable: false,
        symbol: {
          markerFile: iconCache[entity.sidc],
          markerDy: 10,
        },
      });
      marker.on("click", () => setSelectedEntityId(entity.id));
      iconLayer.addGeometry(marker);
    } else {
      marker.setCoordinates([entity.longitude, entity.latitude]);
    }
  }, [entity]);

  return <></>;
}
